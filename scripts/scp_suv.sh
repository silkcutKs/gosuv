#!/usr/bin/env bash

if [ "$#" -ne 1 ]; then
    echo "Please input hostname"
    exit -1
fi

host_name=$1

contains () {
    local list=$1[@]
    local elem=$2
    for i in "${!list}"; do
        if [ "$i" == "${elem}" ] ; then
            return 0
        fi
    done
    return 1
}


# 更新配置
ssh root@${host_name} 'mkdir -p /usr/local/gosuv/res'
ssh root@${host_name} 'mkdir -p /usr/local/gosuv/templates'
ssh root@${host_name} 'chown -R worker.worker /usr/local/gosuv/'

scp -r conf/config.${host_name}.yml root@${host_name}:/usr/local/gosuv/config.yml

# 同步资源文件
rsync -avp res/. ${host_name}:/usr/local/gosuv/res
rsync -avp templates/. ${host_name}:/usr/local/gosuv/templates


# 更新improxy
ssh root@${host_name} "rm -rf /usr/local/gosuv/tool_gosuv"
scp tool_gosuv root@${host_name}:/usr/local/gosuv/tool_gosuv


go_suv_root_hosts=("hosta" "hostb")

# 拷贝systemctl
if contains go_suv_root_hosts $host_name; then
    scp scripts/gosuv.root.service root@${host_name}:/lib/systemd/system/gosuv.service
else
    scp scripts/gosuv.service root@${host_name}:/lib/systemd/system/gosuv.service
fi


# 创建工作目录
ssh root@${host_name} "mkdir -p /data/logs/"
ssh root@${host_name} "chown worker.worker /data/logs/"

# 启动服务
ssh root@${host_name} "systemctl daemon-reload"
ssh root@${host_name} "systemctl enable gosuv"
ssh root@${host_name} "systemctl restart gosuv"

