#!/bin/sh
set -e
if [ -f $WORKSPACE/../TOGGLE ]; then
    echo "****************************************************"
    echo "data.stack:b2bgw :: Toggle mode is on, terminating build"
    echo "data.stack:b2bgw :: BUILD CANCLED"
    echo "****************************************************"
    exit 0
fi

cDate=`date +%Y.%m.%d.%H.%M` #Current date and time

if [ -f $WORKSPACE/../CICD ]; then
    CICD=`cat $WORKSPACE/../CICD`
fi
if [ -f $WORKSPACE/../DATA_STACK_RELEASE ]; then
    REL=`cat $WORKSPACE/../DATA_STACK_RELEASE`
fi
if [ -f $WORKSPACE/../DOCKER_REGISTRY ]; then
    DOCKER_REG=`cat $WORKSPACE/../DOCKER_REGISTRY`
fi
BRANCH='3.0'
if [ -f $WORKSPACE/../BRANCH ]; then
    BRANCH=`cat $WORKSPACE/../BRANCH`
fi
if [ $1 ]; then
    REL=$1
fi
if [ ! $REL ]; then
    echo "****************************************************"
    echo "data.stack:b2bgw :: Please Create file DATA_STACK_RELEASE with the releaese at $WORKSPACE or provide it as 1st argument of this script."
    echo "data.stack:b2bgw :: BUILD FAILED"
    echo "****************************************************"
    exit 0
fi
TAG=$REL
if [ $2 ]; then
    TAG=$TAG"-"$2
fi
if [ $3 ]; then
    BRANCH=$3
fi
if [ $CICD ]; then
    echo "****************************************************"
    echo "data.stack:b2bgw :: CICI env found"
    echo "****************************************************"
    TAG=$TAG"_"$cDate
    if [ ! -f $WORKSPACE/../DATA_STACK_NAMESPACE ]; then
        echo "****************************************************"
        echo "data.stack:b2bgw :: Please Create file DATA_STACK_NAMESPACE with the namespace at $WORKSPACE"
        echo "data.stack:b2bgw :: BUILD FAILED"
        echo "****************************************************"
        exit 0
    fi
    DATA_STACK_NS=`cat $WORKSPACE/../DATA_STACK_NAMESPACE`
fi

sh $WORKSPACE/scripts/prepare_yaml.sh $REL $2

echo "****************************************************"
echo "data.stack:b2bgw :: Using build :: "$TAG
echo "****************************************************"

echo "****************************************************"
echo "data.stack:b2bgw :: Adding IMAGE_TAG in Dockerfile :: "$TAG
echo "****************************************************"
sed -i.bak s#__image_tag__#$TAG# Dockerfile

if [ -f $WORKSPACE/../CLEAN_BUILD_B2BGW ]; then
    echo "****************************************************"
    echo "data.stack:b2bgw :: Doing a clean build"
    echo "****************************************************"

    docker build --no-cache -t data.stack:b2bgw.$TAG .
    rm $WORKSPACE/../CLEAN_BUILD_B2BGW

    echo "****************************************************"
    echo "data.stack:b2bgw :: Copying deployment files"
    echo "****************************************************"

    if [ $CICD ]; then
        sed -i.bak s#__docker_registry_server__#$DOCKER_REG# b2bgw.yaml
        sed -i.bak s/__release_tag__/"'$REL'"/ b2bgw.yaml
        sed -i.bak s#__release__#$TAG# b2bgw.yaml
        sed -i.bak s#__namespace__#$DATA_STACK_NS# b2bgw.yaml
        sed -i.bak '/imagePullSecrets/d' b2bgw.yaml
        sed -i.bak '/- name: regsecret/d' b2bgw.yaml

        kubectl delete deploy b2bgw -n $DATA_STACK_NS || true # deleting old deployement
        kubectl delete service b2bgw -n $DATA_STACK_NS || true # deleting old service
        kubectl delete service b2bgw-internal -n $DATA_STACK_NS || true # deleting old service
        #creating b2bgww deployment
        kubectl create -f b2bgw.yaml
    fi

else
    echo "****************************************************"
    echo "data.stack:b2bgw :: Doing a normal build"   
    echo "****************************************************"
    docker build -t data.stack:b2bgw.$TAG .
    if [ $CICD ]; then
        if [ $DOCKER_REG ]; then
            kubectl set image deployment/b2bgw b2bgw=$DOCKER_REG/data.stack:b2bgw.$TAG -n $DATA_STACK_NS --record=true
        else 
            kubectl set image deployment/b2bgw b2bgw=data.stack:b2bgw.$TAG -n $DATA_STACK_NS --record=true
        fi
    fi
fi
if [ $DOCKER_REG ]; then
    echo "****************************************************"
    echo "data.stack:b2bgw :: Docker Registry found, pushing image"
    echo "****************************************************"

    docker tag data.stack:b2bgw.$TAG $DOCKER_REG/data.stack:b2bgw.$TAG
    docker push $DOCKER_REG/data.stack:b2bgw.$TAG
fi
echo "****************************************************"
echo "data.stack:b2bgw :: BUILD SUCCESS :: data.stack:b2bgw.$TAG"
echo "****************************************************"
echo $TAG > $WORKSPACE/../LATEST_B2BGW
