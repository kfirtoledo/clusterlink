import os,time
import subprocess as sp
import sys
import argparse

proj_dir = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname( os.path.abspath(__file__)))))

sys.path.insert(0,f'{proj_dir}/tests/')
print(f"{proj_dir}/tests/")
from aux.kindAux import runcmd, runcmdb, printHeader, waitPod, getPodName, getKindIp, getMbgPorts,buildMbg,buildMbgctl

def iperf3Test(cmd):
    print(cmd)
    direct_output = sp.check_output(cmd,shell=True) #could be anything here.  
    printHeader(f"Iperf3 Test Results:\n") 
    print(f"{direct_output.decode()}")
    print("***************************************")
    if "iperf Done" in direct_output.decode():
        print(f'Test Pass')
    else:
        print(f'Test Fail')
    print("***************************************")


############################### MAIN ##########################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Description of your program')
    parser.add_argument('-d','--dataplane', help='choose which dataplane to use mtls/tcp', required=False, default="tcp")
    args = vars(parser.parse_args())

    printHeader("\n\nStart Kind Test\n\n")
    printHeader("Start pre-setting")

    
    mbg1DataPort= "30001"
    mbg1cPort= "30443"
    mbg1cPortLocal= "8443"
    
    mbg2DataPort= "30001"
    mbg2cPort= "30443"
    mbg2cPortLocal= "8443"

    srcSvc ="iperfisrael"
    srcDefaultGW="10.244.0.1"
    srcIp=":5000"

    destSvc ="iperfIndia"
    iperf3DestPort="30001"

    dataplane = args["dataplane"]
    mbg1crtFlags =f"--rootCa ./mtls/ca.crt --certificate ./mtls/mbg1.crt --key ./mtls/mbg1.key"  if dataplane =="mtls" else ""
    mbg2crtFlags =f"--rootCa ./mtls/ca.crt --certificate ./mtls/mbg2.crt --key ./mtls/mbg2.key"  if dataplane =="mtls" else ""
    hostcrtFlags =f"--rootCa ./mtls/ca.crt --certificate ./mtls/mbg1.crt --key ./mtls/mbg1.key"  if dataplane =="mtls" else ""
    destcrtFlags =f"--rootCa ./mtls/ca.crt --certificate ./mtls/mbg2.crt --key ./mtls/mbg2.key"  if dataplane =="mtls" else ""

    print(f'Working directory {proj_dir}')
    os.chdir(proj_dir)
    ### clean 
    print(f"Clean old kinds")
    os.system("make clean-kind-iperf3")
    
    ### build docker environment 
    printHeader(f"Build docker image")
    os.system("make docker-build")
    ###Run first Mbg
    printHeader("\n\nStart building MBG1")
    podMbg1, mbg1Ip= buildMbg("mbg-agent1",f"{proj_dir}/manifests/kind/mbg-config1.yaml")
    runcmdb(f'kubectl exec -i {podMbg1} -- ./mbg start --id "MBG1" --ip {mbg1Ip} --cport {mbg1cPort} --cportLocal {mbg1cPortLocal}  --externalDataPortRange {mbg1DataPort}\
    --dataplane {args["dataplane"]} {mbg1crtFlags}')
    runcmd(f"kubectl create service nodeport mbg --tcp={mbg1cPortLocal}:{mbg1cPortLocal} --node-port={mbg1cPort}")

    ###Run Second Mbg
    printHeader("\n\nStart building MBG2")
    podMbg2, mbg2Ip= buildMbg("mbg-agent2",f"{proj_dir}/manifests/kind/mbg-config2.yaml")
    runcmdb(f'kubectl exec -i {podMbg2} -- ./mbg start --id "MBG2" --ip {mbg2Ip} --cport {mbg2cPort} --cportLocal {mbg2cPortLocal} --externalDataPortRange {mbg2DataPort} \
    --dataplane {args["dataplane"]} {mbg2crtFlags}')
    
    runcmd(f"kubectl create service nodeport mbg --tcp={mbg2cPortLocal}:{mbg2cPortLocal} --node-port={mbg2cPort}")
    
    ###Run host
    printHeader("\n\nStart building host-cluster")
    folCl=f"{proj_dir}/tests/iperf3/manifests/iperf3-client"
    runcmd(f"kind create cluster --config {folCl}/kind-config.yaml --name=host-cluster")
    runcmd(f"kind load docker-image mbg --name=host-cluster")
    runcmd(f"kubectl create -f {folCl}/iperf3-client.yaml")
    runcmd(f"kubectl create -f {folCl}/iperf3-svc.yaml")
    podhost, hostIp= buildMbgctl("host Cluster")
    runcmdb(f'kubectl exec -i {podhost} -- ./mbgctl start --id "hostCluster"  --ip {hostIp} --mbgIP {mbg1Ip}:{mbg1cPort}  --dataplane {args["dataplane"]} {hostcrtFlags} ')
    printHeader(f"Add {srcSvc} (client) service to host cluster")
    runcmd(f'kubectl exec -i {podhost} -- ./mbgctl addService --serviceId {srcSvc} --serviceIp {srcDefaultGW}')

    # Add MBG Peer
    printHeader("Add MBG2 peer to MBG1")
    runcmd(f'kubectl exec -i {podhost} -- ./mbgctl addPeer --id "MBG2" --ip {mbg2Ip} --cport {mbg2cPort}')
    
    # Send Hello
    printHeader("Send Hello commands")
    runcmd(f'kubectl exec -i {podhost} -- ./mbgctl hello')
    
    ##Run dest
    printHeader("\n\nStart building dest-cluster")
    folSv=f"{proj_dir}/tests/iperf3/manifests/iperf3-server"
    runcmd(f"kind create cluster --config {folSv}/kind-config.yaml --name=dest-cluster")
    runcmd(f"kind load docker-image mbg --name=dest-cluster")
    runcmd(f"kubectl create -f {folSv}/iperf3.yaml")
    podest, destIp= buildMbgctl("dest Cluster")   
    runcmd(f"kubectl create service nodeport iperf3-server --tcp=5000:5000 --node-port={iperf3DestPort}")
    runcmdb(f'kubectl exec -i {podest} -- ./mbgctl start --id "destCluster"  --ip {destIp}  --mbgIP {mbg2Ip}:{mbg2cPort} --dataplane {args["dataplane"]} {destcrtFlags}')
    printHeader(f"Add {destSvc} (server) service to destination cluster")
    runcmd(f'kubectl exec -i {podest} -- ./mbgctl addService --serviceId {destSvc} --serviceIp {destIp}:{iperf3DestPort}')

    #Add host cluster to MBG1
    runcmd(f'kubectl config use-context kind-mbg-agent1')
    printHeader("Add host cluster to MBG1")
    runcmd(f'kubectl exec -i {podMbg1} -- ./mbg addMbgctl --id "hostCluster" --ip {hostIp}:30000')

    #Add dest cluster to MBG2
    runcmd(f'kubectl config use-context kind-mbg-agent2')
    printHeader("Add dest cluster to MBG2")
    runcmd(f'kubectl exec -i {podMbg2} -- ./mbg addMbgctl --id "destCluster" --ip {destIp}:30000')

    #Expose destination service
    runcmd(f'kubectl config use-context kind-dest-cluster')
    printHeader("\n\nStart exposing connection")
    runcmdb(f'kubectl exec -i {podest} -- ./mbgctl expose --serviceId {destSvc}')

    #Get services
    runcmd(f'kubectl config use-context kind-host-cluster')
    printHeader("\n\nStart get service")
    runcmdb(f'kubectl exec -i {podhost} -- ./mbgctl getService')
    # Create Nodeports inside mbg1
    runcmd(f'kubectl config use-context kind-mbg-agent1')
    mbg1LocalPort, mbg1ExternalPort = getMbgPorts(podMbg1, srcSvc,destSvc)
    runcmd(f"kubectl create service nodeport {srcSvc} --tcp={mbg1LocalPort}:{mbg1LocalPort} --node-port={mbg1ExternalPort}")
    runcmd(f"kubectl patch service {srcSvc} -p "+  "\'{\"spec\":{\"selector\":{\"app\": \"mbg\"}}}\'") #replacing app name

    # Create connect from cluster to MBG
    printHeader(f"\n\nStart Data plan connection {srcSvc} to {destSvc}")
    runcmd(f'kubectl config use-context kind-host-cluster')
    runcmdb(f'kubectl exec -i {podhost} -- ./mbgctl connect --serviceId {srcSvc} --serviceIp {srcIp} --serviceIdDest {destSvc}')

    #Testing
    printHeader("\n\nStart Iperf3 testing")
    runcmd(f'kubectl config use-context kind-dest-cluster')
    waitPod("iperf3-server")
    runcmd(f'kubectl config use-context kind-host-cluster')
    waitPod("iperf3-client")
    podIperf3= getPodName("iperf3-clients")

    printHeader("The Iperf3 test connects directly to the destination")
    cmd = f'kubectl exec -i {podIperf3} --  iperf3 -c {destIp} -p {iperf3DestPort}'
    iperf3Test(cmd)

    printHeader("The Iperf3 test connects to MBG1")
    cmd = f'kubectl exec -i {podIperf3} --  iperf3 -c {mbg1Ip} -p {mbg1ExternalPort}'
    iperf3Test(cmd)

    printHeader("Full Iperf3 test clinet-> MBG1-> MBG2-> dest")
    cmd = f'kubectl exec -i {podIperf3} --  iperf3 -c mbgctl-iperf3-service -p 5000'
    iperf3Test(cmd)

    #Close connection
    printHeader("\n\nClose Iperf3 connection")
    runcmd(f'kubectl exec -i {podhost} -- ./mbgctl disconnect --serviceId {srcSvc} --serviceIdDest {destSvc}')