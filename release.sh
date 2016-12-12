#!/bin/bash
#
# Maintainer script for publishing releases.

set -e

source=$(dpkg-parsechangelog -S Source)
version=$(dpkg-parsechangelog -S Version)
arm_target="robot@ev3dev-rpi3"

OS=debian DIST=jessie ARCH=amd64 pbuilder-ev3dev build
OS=debian DIST=jessie ARCH=i386 PBUILDER_OPTIONS="--binary-arch" pbuilder-ev3dev build

ssh ${arm_target} "mkdir -p ~/pbuilder-ev3dev/source"
scp ~/pbuilder-ev3dev/debian/jessie-amd64/${source}_${version}.dsc \
    ${arm_target}:~/pbuilder-ev3dev/source
scp ~/pbuilder-ev3dev/debian/jessie-amd64/${source}_${version%-*}.orig.tar.gz \
    ${arm_target}:~/pbuilder-ev3dev/source
scp ~/pbuilder-ev3dev/debian/jessie-amd64/${source}_${version}.debian.tar.xz \
    ${arm_target}:~/pbuilder-ev3dev/source

ssh -t ${arm_target} "OS=debian DIST=jessie ARCH=armhf PBUILDER_OPTIONS='--binary-arch' pbuilder-ev3dev dsc-build \
    ~/pbuilder-ev3dev/source/${source}_${version}.dsc"
mkdir -p ~/pbuilder-ev3dev/debian/jessie-armhf
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armhf/lmsasm_${version}_armhf.deb \
    ~/pbuilder-ev3dev/debian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armhf/lmsgen_${version}_armhf.deb \
    ~/pbuilder-ev3dev/debian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armhf/${source}_${version}_armhf.changes \
    ~/pbuilder-ev3dev/debian/jessie-armhf/

ssh -t ${arm_target} "OS=debian DIST=jessie ARCH=armel PBUILDER_OPTIONS='--binary-arch' pbuilder-ev3dev dsc-build \
    ~/pbuilder-ev3dev/source/${source}_${version}.dsc"
mkdir -p ~/pbuilder-ev3dev/debian/jessie-armel
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armel/lmsasm_${version}_armel.deb \
    ~/pbuilder-ev3dev/debian/jessie-armel/
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armel/lmsgen_${version}_armel.deb \
    ~/pbuilder-ev3dev/debian/jessie-armel/
scp ${arm_target}:~/pbuilder-ev3dev/debian/jessie-armel/${source}_${version}_armel.changes \
    ~/pbuilder-ev3dev/debian/jessie-armel/

ssh -t ${arm_target} "OS=raspbian DIST=jessie ARCH=armhf pbuilder-ev3dev dsc-build \
    ~/pbuilder-ev3dev/source/${source}_${version}.dsc"
mkdir -p ~/pbuilder-ev3dev/raspbian/jessie-armhf
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version}.dsc \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version%-*}.orig.tar.gz \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version}.debian.tar.xz \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/lmsasm_${version}_armhf.deb \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/lmsgen_${version}_armhf.deb \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/
scp ${arm_target}:~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version}_armhf.changes \
    ~/pbuilder-ev3dev/raspbian/jessie-armhf/

debsign ~/pbuilder-ev3dev/debian/jessie-amd64/${source}_${version}_amd64.changes
debsign ~/pbuilder-ev3dev/debian/jessie-i386/${source}_${version}_i386.changes
debsign ~/pbuilder-ev3dev/debian/jessie-armhf/${source}_${version}_armhf.changes
debsign ~/pbuilder-ev3dev/debian/jessie-armel/${source}_${version}_armel.changes
debsign ~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version}_armhf.changes

dput ev3dev-debian ~/pbuilder-ev3dev/debian/jessie-amd64/${source}_${version}_amd64.changes
dput ev3dev-debian ~/pbuilder-ev3dev/debian/jessie-i386/${source}_${version}_i386.changes
dput ev3dev-debian ~/pbuilder-ev3dev/debian/jessie-armhf/${source}_${version}_armhf.changes
dput ev3dev-debian ~/pbuilder-ev3dev/debian/jessie-armel/${source}_${version}_armel.changes
dput ev3dev-raspbian ~/pbuilder-ev3dev/raspbian/jessie-armhf/${source}_${version}_armhf.changes

gbp buildpackage --git-tag-only
