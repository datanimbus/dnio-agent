#!/bin/sh

echo "****************************************************"
echo "data.stack:b2bgw :: Copying yaml file "
echo "****************************************************"
if [ ! -d $WORKSPACE/../yamlFiles ]; then
    mkdir $WORKSPACE/../yamlFiles
fi

REL=$1
if [ $2 ]; then
    REL=$REL-$2
fi

rm -rf $WORKSPACE/../yamlFiles/b2bgw.* $WORKSPACE/../yamlFiles/b2bgw.*
cp $WORKSPACE/b2bgw.yaml $WORKSPACE/../yamlFiles/b2bgw.$REL.yaml
cd $WORKSPACE/../yamlFiles/
echo "****************************************************"
echo "data.stack:b2bgw :: Preparing yaml file "
echo "****************************************************"
sed -i.bak s/__release_tag__/"'$1'"/ b2bgw.$REL.yaml
sed -i.bak s/__release__/$REL/ b2bgw.$REL.yaml
