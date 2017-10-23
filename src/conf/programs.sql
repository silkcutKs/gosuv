CREATE DATABASE gosuv_db character set utf8mb4;
CREATE TABLE `programs` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `host` varchar(100) DEFAULT NULL,
  `name` varchar(100) DEFAULT NULL,
  `command` varchar(500) DEFAULT NULL,
  `environ_db` varchar(2000) DEFAULT NULL,
  `dir` varchar(255) DEFAULT NULL,
  `start_auto` tinyint(1) DEFAULT NULL,
  `start_retries` int(11) DEFAULT NULL,
  `start_seconds` int(11) DEFAULT NULL,
  `stop_timeout` int(11) DEFAULT NULL,
  `user` varchar(40) DEFAULT NULL,
  `author`  varchar(40) DEFAULT NULL,
  `process_num` int(11) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_host_name` (`host`,`name`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8;