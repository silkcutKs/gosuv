# 概述
 * gosuv是GO语言重写的类supervisor的一个进程管理程序，简单易用，界面美感十足且对用户友好 

## 使用
* 启动服务
    * ` ./tool_gosuv -c config.yml start`

* 查看服务状态
    * `./tool_gosuv -c config.yml status`

* 默认端口 11113  本机测试请使用[http://localhost:11313](http://localhost:11313)
    * 可能开启ldap支持
![RunImage](docs/gosuv.gif)

## 配置
 * 配置也可以放在数据库中, 否则使用默认的配置文件
 * 项目文件名 ：     programs.yml
 * 服务器配置文件名：    config.yml

## 验证信息配置

```yml
server:
  ldap:
    enabled: true
    host: xxxx
    base: ou=xxx,dc=test,dc=org
    port: 10489
    use_ssl: false
    bind_dn: uid=bind,ou=test,dc=org,dc=xx
    bind_password: xxx
    user_filter: (uid=%s)
    attributes:
    - givenName
    - sn
    - mail
    - uid
  addr: :11313
client:
  server_url: http://localhost:11313
db:
  db_type: mysql
  db_dsn: root:@tcp(localhost:3306)/log?tls=skip-verify&autocommit=true
host: wfmac
admins:
- xiaogao
- xiaohe
```

## 日志文件
* 日志的使用: `./tool_gosuv -c config.yml start -L /data/logs/service.log`
* 实际的日志：
    * gosuv日志: /data/logs/service.log-20170617
    * 程序日志:  /data/logs/event_trigger.log-20170617

## 权限管理
* 每个Program都绑定一个Author, 只有Author和amdins可以对该Program进行管理和重启

## 程序运行理念
* graceful stop/restart: 进程收到SIGTERM之后需要尽快完成手上的工作，然后退出
    * 避免无限制地等待，例如：
        * beanstalk->reserve() --> beanstalk->reserve(5)
        * redis->brpop() --> redis->brpop(5)
        * sleep(100) --> while(!notSleep) { sleep(5);}
    * 等等
* /usr/local/service/gosuv/tool_gosuv -c /usr/local/service/gosuv/config.yml restart