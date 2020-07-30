package manager

import (
	"os"

	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
)

const (
	DefaultMemManagerBaseDir = "/root/fccd-mem_manager"
)

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	RecordReplayModeEnabled bool
	MemManagerBaseDir string
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	sync.Mutex
	inactive map[string]*SnapshotState
	activeFdState map[int]*SnapshotState // indexed by FD
	activeVmFd   map[string]int            // Indexed by vmID
	epfd         int
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(quitCh chan int) *MemoryManager {
	log.Debug("Inializing the memory manager")

	v := new(MemoryManager)
	v.inactive = make(map[string]*SnapshotState)
	v.activeFdState = make(map[int]*SnapshotState)
	v.activeVmFd = make(map[string]int)


	// start the main (polling) loop in a goroutine
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L409

	// use select + cases to execute the infinite loop and also wait for a
	// message on quitCh channel to terminate the main loop
	readyCh := make(chan int)
	v.pollingLoop(readyCh, quitCh)

	<-readyCh

	return v
}

// RegisterVM Register a VM which is going to be
// managed by the memory manager
func (v *MemoryManager) RegisterVM(cfg *SnapshotStateCfg) error {
	v.Lock()
	defer v.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Registering VM with the memory manager")

	if _, ok := v.inactive[vmID]; ok {
		logger.Error("VM already registered the memory manager")
		return errors.New("VM exists in the memory manager")
	}

	if _, ok := v.activeVmFd[vmID]; ok {
		logger.Error("VM already active in the memory manager")
		return errors.New("VM already active in the memory manager")
	}

	state := NewSnapshotState(cfg)

	v.inactive[vmID] = state

	return nil
}

// AddInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) AddInstance(vmID) (err error) {
	v.Lock()
	defer v.Unlock()
	
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Adding instance to the memory manager")

	var (
		event syscall.EpollEvent
		fdInt    int
	)

	if _, ok := v.inactive[vmID]; !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	if _, ok := v.vmFdMap[vmID]; ok {
		logger.Error("VM exists in the memory manager")
		return errors.New("VM exists in the memory manager")
	}

	if err := state.mapGuestMemory(); err != nil {
		logger.Error("Failed to map guest memory")
		return err
	}

	state.getUFFD()

	fdInt = int(state.UserFaultFD.Fd())

	delete(v.inactive, vmID)
	v.activeVmFd[vmID] = fdInt
	v.activeFdState[fdInt] = state

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fdInt)

	if err := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		logger.Error("Failed to subscribe VM")
		return err
	}

	return
}

// RemoveInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) RemoveInstance(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Removing instance from the memory manager")

	var (
		state SnapshotState
		fdInt    int
		ok    bool
	)

	if _, ok := v.inactive[vmID]; !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	fdInt, ok = v.vmFdMap[vmID]
	if !ok {
		logger.Error("Failed to find fd")
		return errors.New("Failed to find fd")
	}

	state, ok = v.snapStateMap[fdInt]
	if !ok {
		logger.Error("Failed to find snapshot state")
		return errors.New("Failed to find snapshot state")
	}

	if err := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_DEL, fdInt, &event); e != nil {
		logger.Error("Failed to unsubscribe VM")
		return err
	}

	// munmap the guest memory file
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L403
	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.UserFaultFD.Close()

	delete(v.snapStateMap, fdInt)
	delete(v.vmFdMap, vmId)
	v.inactive = state

	return nil
}

// FetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (v *MemoryManager) FetchState(vmID string) (err error) {
	// NOT IMPLEMENTED
	return nil
}

func (v *MemoryManager) pollingLoop(readyCh, quitCh chan int) {
	var (
		events [1000]syscall.EpollEvent
		err error
		servedNum   int
		startAddress uint64
	)

	v.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		log.Fatalf("epoll_create1: %v", err)
		os.Exit(1)
	}
	defer syscall.Close(v.epfd)

	close(readyCh)

	for {
		select {
		case <-quitCh:
			log.Debug("Handler received a signal to quit")
			return
		default:
			nevents, e := syscall.EpollWait(v.epfd, events[:], -1)
			if e != nil {
				log.Fatalf("epoll_wait: %v", e)
				break
			}
			if nevents < 1 {
				panic("Wrong number of events")
			}

			for _, event := range events {
				fd := event.Fd
				_, ok := v.activeFdState[fd]
				if !ok {
					log.Fatalf("received event from file which is not active")
				}

				address := extractPageFaultAddress(fd)

				state := v.getSnapshotState(fd)
				state.startAddressOnce.Do(
					func() {
						state.startAddress = address
					}
				)
				go v.servePageFault(fd, address)
			}
		}
	}
}


func installRegion(fd int, src, dst, mode, len uint64) error {
	cUC := C.struct_uffdio_copy{
		mode: C.ulonglong(mode),
		copy: 0,
		src:  C.ulonglong(src),
		dst:  C.ulonglong(dst),
		len:  C.ulonglong(pageSize * len),
	}

	err := ioctl(fd.Fd(), int(C.const_UFFDIO_COPY), unsafe.Pointer(&cUC))
	if err != nil {
		return err
	}

	return nil
}

func (v *MemoryManager) servePageFault(fd int, address uint64) {
	snapState := v.getSnapshotState(fd)
	offset := address - state.startAddress

	src := uint64(uintptr(unsafe.Pointer(&state.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(pageSize) - 1))
	mode := uint64(0)

	installRegion(fd, src, dst, mode, 1)
}


func (v *MemoryManager) extractPageFaultAddress(fd int) uint64 {
	goMsg := make([]byte, C.sizeof_struct_uffd_msg)
	if nread, err := syscall.Read(fd, goMsg); err != nil || nread != len(goMsg) {
		log.Fatalf("Read uffd_msg failed: %v", err)
	}

	if event := uint8(goMsg[0]); event != uint8(C.const_UFFD_EVENT_PAGEFAULT) {
		log.Fatal("Received wrong event type")
	}

	return binary.LittleEndian.Uint64(goMsg[16:])
}

func (v *MemoryManager) getSnapshotState(fd int) *SnapshotState {
	if state, ok := v.activeFdState[fd]; ok {
		return state
	}
	log.Fatalf("getSnapshotState: fd not found")
}

func ioctl(fd uintptr, request int, argp unsafe.Pointer) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		fd,
		uintptr(request),
		// Note that the conversion from unsafe.Pointer to uintptr _must_
		// occur in the call expression.  See the package unsafe documentation
		// for more details.
		uintptr(argp),
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", fmt.Errorf("%d", int(errno)))
	}

	return nil
}

func (s *SnapshotState) mapGuestMemory(state *misc.SnapshotState) error {
	fd, err := os.OpenFile(s.guestMemFileName, os.O_RDONLY, 0600)
	if err != nil {
		log.Errorf("Failed to open guest memory file: %v", err)
		return err
	}

	s.guestMem, err = unix.Mmap(int(fd.Fd()), 0, s.guestMemSize, unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		log.Errorf("Failed to mmap guest memory file: %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) unmapGuestMemory() error {
	if err := unix.Munmap(s.guestMem); err != nil {
		log.Errorf("Failed to munmap guest memory file: %v", err)
		return err
	}
	
	return nil
}