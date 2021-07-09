#!/bin/bash

# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Shyam Jesalpura and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

# download and install docker
sudo apt-get update

sudo apt-get install -y \
    jq >> /dev/null

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$PWD/../install_go.sh
$PWD/../install_docker.sh

# install kind from ease-lab/kind
rm -rf /tmp/kind/
git clone -b custom_docker_params_for_vHive https://github.com/ease-lab/kind /tmp/kind/
cd /tmp/kind
source /etc/profile && go build
sudo mv kind /usr/local/bin/

# Allow profiling using Perf / PMU tools
sudo sysctl -w kernel.perf_event_paranoid=-1
