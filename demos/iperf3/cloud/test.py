#!/usr/bin/env python3
# Copyright 2023 The ClusterLink Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

################################################################
#Name: Service node test
#Desc: create 1 proxy that send data to target ip
###############################################################
import os,sys
file_dir = os.path.dirname(__file__)
proj_dir = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname( os.path.abspath(__file__)))))
sys.path.insert(0,f'{proj_dir}')
sys.path.insert(1,f'{proj_dir}/demos/utils/cloud/')


from demos.utils.common import runcmd, createFabric, printHeader, startGwctl, createGw, createPeer, applyPeer
from demos.utils.k8s import  getPodNameIp,waitPod
from demos.utils.cloud import cluster

import argparse

gw1gcp = cluster(name="peer1", zone = "us-west1-b", platform = "gcp", type = "host") 
gw1ibm = cluster(name="peer1", zone = "dal10",      platform = "ibm", type = "host")
gw2gcp = cluster(name="peer2", zone = "us-west1-b", platform = "gcp", type = "target")
gw2ibm = cluster(name="peer2", zone = "dal10",      platform = "ibm", type = "target")

srcSvc           = "iperf3-client"
destSvc          = "iperf3-server"
destPort         = 5000
iperf3DirectPort = "30001"

# Folders
folCl=f"{proj_dir}/demos/iperf3/testdata/manifests/iperf3-client"
folSv=f"{proj_dir}/demos/iperf3/testdata/manifests/iperf3-server"
testOutputFolder = f"{proj_dir}/bin/tests/iperf3"

# Policy
allowAllPolicy=f"{proj_dir}/pkg/policyengine/policytypes/examples/allowAll.json"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Description of your program')
    parser.add_argument('-c','--command', help='Script command: test/delete', required=False, default="test")
    parser.add_argument('-m','--machineType', help='Type of machine to create small/large', required=False, default="small")
    parser.add_argument('-cloud','--cloud', help='Cloud setup using gcp/ibm/diff (different clouds)', required=False, default="gcp")
    parser.add_argument('-delete','--deleteCluster', help='Delete clusters in the end of the test', required=False, default="true")

    args = vars(parser.parse_args())

    command = args["command"]
    cloud = args["cloud"]
    dltCluster = args["deleteCluster"]
    machineType = args["machineType"]
    gw1 = gw1gcp if cloud in ["gcp","diff"] else gw1ibm
    gw2 = gw2gcp if cloud in ["gcp"]        else gw2ibm
    gwctl1 = "gwctl1"
    gwctl2 = "gwctl2"
    gwPort = "443"
    print(f'Working directory {proj_dir}')
    os.chdir(proj_dir)
    
    ### build docker environment 
    printHeader("Build docker image")
    os.system("make build")
    os.system("sudo make install")
    
    if command =="delete":
        gw1.deleteCluster(runBg=True)
        gw2.deleteCluster()

        exit()
    elif command =="clean":
        gw1.cleanCluster()
        gw2.cleanCluster()
        exit()

    #Create k8s cluster
    gw1.createCluster(run_in_bg=True , machineType = machineType)
    gw2.createCluster(run_in_bg=False, machineType = machineType)
    createFabric(testOutputFolder)
    #Setup MBG1
    gw1.checkClusterIsReady()
    createPeer(gw1.name,testOutputFolder)
    gw1.replace_source_image(f"{testOutputFolder}/{gw1.name}/k8s.yaml","quay.io/mcnet/")
    applyPeer(gw1.name,testOutputFolder)
    gw1.createLoadBalancer()
    
    #Build MBG2
    gw2.checkClusterIsReady()
    createPeer(gw2.name,testOutputFolder)
    gw2.replace_source_image(f"{testOutputFolder}/{gw2.name}/k8s.yaml","quay.io/mcnet/")
    applyPeer(gw2.name,testOutputFolder)
    gw2.createLoadBalancer()
    
    # Start gwctl
    startGwctl(gw1.name, gw1.ip, gw1.port, testOutputFolder)
    startGwctl(gw2.name, gw2.ip, gw2.port, testOutputFolder)
    
    # Create peers
    printHeader("Create peers")
    runcmd(f'gwctl create peer --myid {gw1.name} --name {gw2.name} --host {gw2.ip} --port {gwPort}')
    runcmd(f'gwctl create peer --myid {gw2.name} --name {gw1.name} --host {gw1.ip} --port {gwPort}')
    
    # Set service iperf3-client in gw1
    gw1.connectToCluster()
    runcmd(f"kubectl create -f {folCl}/iperf3-client.yaml")
    waitPod(srcSvc)
    runcmd(f'gwctl create export --myid {gw1.name} --name {srcSvc} --host {srcSvc} --port {destPort}')

    # Set service iperf3-server in gw2
    gw2.connectToCluster()
    runcmd(f"kubectl create -f {folSv}/iperf3.yaml")
    waitPod(destSvc)
    runcmd(f'gwctl create export --myid {gw2.name} --name {destSvc} --host {destSvc} --port {destPort}')

    #Import destination service
    printHeader(f"\n\nStart Importing {destSvc} service to {gw1.name}")
    runcmd(f'gwctl --myid {gw1.name} create import --name {destSvc} --host {destSvc} --port {destPort}')
    printHeader(f"\n\nStart binding {destSvc} service to {gw1.name}")
    runcmd(f'gwctl --myid {gw1.name} create binding --import {destSvc} --peer {gw2.name}')

    #Add policy
    printHeader("Applying policies")
    runcmd(f'gwctl --myid {gw1.name} create policy --type access --policyFile {allowAllPolicy}')
    runcmd(f'gwctl --myid {gw2.name} create policy --type access --policyFile {allowAllPolicy}')

    #Test MBG1
    gw1.connectToCluster()
    podIperf3,_= getPodNameIp(srcSvc)

    for i in range(2):
        printHeader(f"iPerf3 test {i}")
        cmd = f'kubectl exec -i {podIperf3} --  iperf3 -c iperf3-server -p {5000} -t 40'
        runcmd(cmd)

    ## clean target and source clusters
    # delete_all_clusters()