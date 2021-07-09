# Self-Hosted Stock-Knative on KinD Runners

## Setup
1. Install Ansible on your local machine: \
   https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html#installing-ansible-on-specific-operating-systems
2. Start a new experiment on Cloudlab:
    - Profile: `small-lan`
    - OS Image: `UBUNTU 18.04`
    - Physical Node Type: `rs440` (others are probably fine too)
    - (Under "Advanced") Temp Filesystem Max Space: Selected
3. On your local machine, use Ansible to setup the remote host:
   ```bash
   ansible-playbook -u <YOUR SSH USERNAME> -i <HOSTNAME>, setup-host.yaml
   ```
4. On your local machine, use Ansible to create a stock-Knative runner on KinD:
   ```bash
   ansible-playbook -u <YOUR SSH USERNAME> -i <HOSTNAME>, playbook.yaml
   ```

You may call the fourth step as many times as you like.

## Destroying
```bash
kind delete cluster --name <CLUSTER NAME>
```

- `<CLUSTER NAME>` is a random predicate-object tuple, such as `thankful-magnolia`.
- "A self-hosted runner is automatically removed from GitHub if it has not connected
  to GitHub Actions for more than 30 days." ([GitHub](https://docs.github.com/en/actions/hosting-your-own-runners/removing-self-hosted-runners#removing-a-runner-from-a-repository))

  Do not worry about stale entries.
