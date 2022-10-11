#!/bin/sh
set -e
if [ -f $WORKSPACE/../TOGGLE ]; then
    echo "****************************************************"
    echo "data.stack.b2b.agents :: Toggle mode is on, terminating build"
    echo "data.stack.b2b.agents :: BUILD CANCLED"
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
    echo "data.stack.b2b.agents :: Please Create file DATA_STACK_RELEASE with the releaese at $WORKSPACE or provide it as 1st argument of this script."
    echo "data.stack.b2b.agents :: BUILD FAILED"
    echo "****************************************************"
    exit 0
fi
TAG=$REL
# if [ $2 ]; then
#     TAG=$TAG"-"$2
# fi
if [ $3 ]; then
    BRANCH=$3
fi
# if [ $CICD ]; then
#     echo "****************************************************"
#     echo "data.stack.b2b.agents :: CICI env found"
#     echo "****************************************************"
#     # TAG=$TAG"_"$cDate
#     if [ ! -f $WORKSPACE/../DATA_STACK_NAMESPACE ]; then
#         echo "****************************************************"
#         echo "data.stack.b2b.agents :: Please Create file DATA_STACK_NAMESPACE with the namespace at $WORKSPACE"
#         echo "data.stack.b2b.agents :: BUILD FAILED"
#         echo "****************************************************"
#         exit 0
#     fi
#     DATA_STACK_NS=`cat $WORKSPACE/../DATA_STACK_NAMESPACE`
# fi

# sh $WORKSPACE/scripts/prepare_yaml.sh $REL $2

echo "****************************************************"
echo "data.stack.b2b.agents :: Using build :: "$TAG
echo "****************************************************"

echo "****************************************************"
echo "data.stack.b2b.agents :: Adding IMAGE_TAG in Dockerfile :: "$TAG
echo "****************************************************"
sed -i.bak s#__image_tag__#$TAG# Dockerfile
sed -i.bak s#__signing_key_user__#$SIGNING_KEY_USER# Dockerfile
sed -i.bak s#__signing_key_password__#$SIGNING_KEY_PASSWORD# Dockerfile

if [ -f $WORKSPACE/../CLEAN_BUILD_AGENT ]; then
    echo "****************************************************"
    echo "data.stack.b2b.agents :: Doing a clean build"
    echo "****************************************************"

    docker build --no-cache -t data.stack.b2b.agents.$TAG .
    rm $WORKSPACE/../CLEAN_BUILD_AGENT

    # echo "****************************************************"
    # echo "data.stack.b2b.agents :: Copying deployment files"
    # echo "****************************************************"

    # if [ $CICD ]; then
    #     sed -i.bak s#__docker_registry_server__#$DOCKER_REG# agent.yaml
    #     sed -i.bak s/__release_tag__/"'$REL'"/ agent.yaml
    #     sed -i.bak s#__release__#$TAG# agent.yaml
    #     sed -i.bak s#__namespace__#$DATA_STACK_NS# agent.yaml
    #     sed -i.bak '/imagePullSecrets/d' agent.yaml
    #     sed -i.bak '/- name: regsecret/d' agent.yaml

    #     kubectl delete deploy agent -n $DATA_STACK_NS || true # deleting old deployement
    #     kubectl delete service agent -n $DATA_STACK_NS || true # deleting old service
    #     kubectl delete service agent-internal -n $DATA_STACK_NS || true # deleting old service
    #     #creating agentw deployment
    #     kubectl create -f agent.yaml
    # fi

else
    echo "****************************************************"
    echo "data.stack.b2b.agents :: Doing a normal build"   
    echo "****************************************************"
    docker build -t data.stack.b2b.agents.$TAG .
    # if [ $CICD ]; then
    #     if [ $DOCKER_REG ]; then
    #         kubectl set image deployment/agent agent=$DOCKER_REG/data.stack.b2b.agents.$TAG -n $DATA_STACK_NS --record=true
    #     else 
    #         kubectl set image deployment/agent agent=data.stack.b2b.agents.$TAG -n $DATA_STACK_NS --record=true
    #     fi
    # fi
fi
if [ $DOCKER_REG ]; then
    echo "****************************************************"
    echo "data.stack.b2b.agents :: Docker Registry found, pushing image"
    echo "****************************************************"

    docker tag data.stack.b2b.agents.$TAG $DOCKER_REG/data.stack.b2b.agents.$TAG
    docker push $DOCKER_REG/data.stack.b2b.agents.$TAG
fi
echo "****************************************************"
echo "data.stack.b2b.agents :: BUILD SUCCESS :: data.stack.b2b.agents.$TAG"
echo "****************************************************"
echo $TAG > $WORKSPACE/../LATEST_AGENT
