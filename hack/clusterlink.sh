#!/bin/sh
set -e

# Detrmine the OS.
OS=$(uname)
if [ "${OS}" = "Darwin" ] ; then
  CL_OS="os"
else
  CL_OS="linux"
fi


# Detrmine the OS architecture.
OS_ARCH=$(uname -m)
case "${OS_ARCH}" in
  x86_64|amd64)
    CL_ARCH=amd64
    ;;
  armv8*|aarch64*|arm64)
    ARCH=arm64
    ;;
  *)
    echo "This ${OS_ARCH} architecture isn't supported"
    exit 1
    ;;
esac

filename="clusterlink-${CL_OS}-${CL_ARCH}.tar.gz"
url="https://github.com/kfirtoledo/clusterlink/releases/download/${VERSION}/${filename}"

# Set version to latest if not define and update url.
if [ "${VERSION}" = "" ] ; then
  VERSION="latest"
  url="https://github.com/kfirtoledo/clusterlink/releases/${VERSION}/download/${filename}"
fi



#url="https://github.com/clusterlinl-net/releases/download/${VERSION}/" +filename
printf "\n Downloading %s from %s ...\n" "$filename" "$url"

if ! curl -o /dev/null -sIf "$url"; then
    printf "\n%s This version of clusterlonk wasn't found\n" "$url"
    exit 1
fi

curl -fsLO ${url}

tar -xzf "${filename}"
rm "${filename}"

path=$(pwd)/clusterlink
printf "\n"
printf "%s has been successfully downloaded into 'clusterlink' folder.\n" "$filename"
printf "\n"
printf "To add ClusterLink CLI to your environment path, use the following command:\n" 
printf "\t export PATH=\"\$PATH:%s\"\n" "$path"
printf "\n"
printf "For more information on how to set up ClusterLink in your Kubernetes cluster, please see: https://cluster-link.net/docs/getting-started \n"
printf "\n"

