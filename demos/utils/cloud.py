import json
import subprocess as sp
import time
import os
from demos.utils.k8s import  cleanCluster
from demos.utils.common import runcmd
import yaml
class cluster:
    def __init__(self, name, zone,platform,type):
        self.name     = name
        self.zone     = zone
        self.platform = platform
        self.type     = type
        self.port     = 443
        self.ip       = ""
    
    def setClusterIP(self):
        clJson=json.loads(sp.getoutput('kubectl get nodes -o json'))
        ip = clJson["items"][0]["status"]["addresses"][1]["address"]
        self.ip = ip
    
    def createCluster(self, run_in_bg, machineType="small"):
        print(f"create {self.name} cluster , zone {self.zone} , platform {self.platform}")
        bg_flags = " &" if run_in_bg else ""
        if self.platform == "gcp":
            flags = "  --machine-type n2-standard-4" if machineType=="large" else "" #e2-medium
            cmd=f"gcloud container clusters create {self.name} --zone {self.zone} --num-nodes 1 --tags tcpall {flags} {bg_flags}"
            print(cmd)
            os.system(cmd)
        elif self.platform == "aws": #--instance-selector-vcpus 2  --instance-selector-memory 4 --instance-selector-cpu-architecture arm64
            cmd =f"eksctl create cluster --name {self.name} --region {self.zone} -N 1  {flags}  {bg_flags}"
            print(cmd)
            os.system(cmd)

        elif self.platform == "ibm":
            vlan_private_ip=sp.getoutput(f"ibmcloud ks vlans --zone {self.zone} |fgrep private |cut -d ' ' -f 1")
            vlan_public_ip=sp.getoutput(f"ibmcloud ks vlans --zone {self.zone}  |fgrep public |cut -d ' ' -f 1")
            print("vlan_public_ip:",vlan_public_ip)
            vlan_private_string = "--private-vlan " + vlan_private_ip  if (vlan_private_ip != "" and "FAILED" not in vlan_private_ip) else ""
            if (vlan_public_ip  != "" and "FAILED" not in vlan_public_ip):
                vlan_public_string  = "--public-vlan "  + vlan_public_ip    
            else:
                vlan_public_string= ""
                vlan_private_string = vlan_private_string + " --private-only " if (vlan_private_string != "") else ""
            
            cmd= f"ibmcloud ks cluster create  classic  --name {self.name} --zone={self.zone} --flavor u3c.2x4 --workers=1 {vlan_private_string} {vlan_public_string} {bg_flags}"
            print(cmd)
            os.system(cmd)
        else:
            print ("ERROR: Cloud platform {} not supported".format(self.platform))    
    
    def connectToCluster(self):
        print(f"\n CONNECT TO: {self.name} in zone: {self.zone} ,platform: {self.platform}\n")
        connect_flag= False
        while (not connect_flag):
            if self.platform == "gcp":
                PROJECT_ID=sp.getoutput("gcloud info --format='value(config.project)'")
                cmd=f"gcloud container clusters  get-credentials {self.name} --zone {self.zone} --project {PROJECT_ID}"
                print(cmd)
            elif self.platform == "aws":
                cmd=f"aws eks --region {self.zone} update-kubeconfig --name {self.name}"
            elif self.platform == "ibm":
                cmd=f"ibmcloud ks cluster config --cluster {self.name}"
            else:
                print (f"ERROR: Cloud platform {self.patform} not supported")
                exit(1)
            
            out_cmd=sp.getoutput(cmd)
            print("connection output: {}".format(out_cmd))
            connect_flag = False if ("ERROR" in out_cmd or "WARNING" in out_cmd or "Failed" in out_cmd) else True
            if not connect_flag: 
                time.sleep(30) #wait more time to connection
            return out_cmd

    def checkClusterIsReady(self):
        connect_flag= False
        while (not connect_flag):
            ret_out=self.connectToCluster()
            connect_flag = False if ("ERROR" in ret_out or "Failed" in ret_out or "FAILED" in ret_out) else True
            time.sleep(20)

        print(f"\n Cluster Ready: {self.name} in zone: {self.zone} ,platform: {self.platform}\n")
        
    #replace the container registry ip according to the proxy platform.
    def replace_source_image(self,yaml_file_path,image_prefix):
        with open(yaml_file_path, 'r') as file:
            yaml_content = file.read()

        # Replace "image:" with the image prefix
        updated_yaml_content = yaml_content.replace("image: ", "image: " + image_prefix)

        # Write the updated YAML content to a new file
        with open(yaml_file_path, 'w') as file:
            file.write(updated_yaml_content)

        print(f"Image prefixes have been added to the updated YAML file: {yaml_file_path}")


    def deleteCluster(self, runBg=False):
        bg_flag= "&" if runBg else ""
        print(f"Deleting cluster {cluster}")
        if self.platform == "gcp" :
            os.system(f"yes |gcloud container clusters delete {self.name} --zone {self.zone} {bg_flag}")
        elif self.platform == "aws":
            os.system(f"eksctl delete cluster --region {self.zone} --name {self.name} {bg_flag}")
        elif self.platform == "ibm":
            os.system(f"yes |ibmcloud ks cluster rm --force-delete-storage --cluster {self.name} {bg_flag}")
        else:
            print ("ERROR: Cloud platform {} not supported".format(self.platform))

    def cleanCluster(self):
        print("Start clean cluster")
        self.connectToCluster()
        cleanCluster()    
            
    def createLoadBalancer(self,port="443", externalIp=""):
        runcmd(f"kubectl expose deployment cl-dataplane --name=cl-dataplane-load-balancer --port={port} --target-port={port} --type=LoadBalancer")
        gwIp=""
        if externalIp !="":
            runcmd("kubectl patch svc cl-dataplane-load-balancer -p "+ "\'{\"spec\": {\"type\": \"LoadBalancer\", \"loadBalancerIP\": \""+ externalIp+ "\"}}\'")
            gwIp= externalIp
        while gwIp =="":
            print("Waiting for cl-dataplane ip...")
            gwIp=sp.getoutput('kubectl get svc -l app=cl-dataplane  -o jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}"')
            time.sleep(10)
        self.ip = gwIp
        return gwIp