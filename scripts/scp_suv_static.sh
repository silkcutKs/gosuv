#!/usr/bin/env bash

if [ "$#" -ne 1 ]; then
    echo "Please input hostname"
    exit -1
fi

host_name=$1


# 更新配置
scp -r conf/config.${host_name}.yml root@${host_name}:/usr/local/gosuv/config.yml

# 同步资源文件
rsync -avp res/. ${host_name}:/usr/local/gosuv/res
rsync -avp templates/. ${host_name}:/usr/local/gosuv/templates

ssh root@${host_name} "chown -R worker.worker /data/logs/"
ssh root@${host_name} "chown -R worker.worker /usr/local/gosuv/"


