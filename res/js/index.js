/* index.js */
var W = {};
var testPrograms = [{
    program: {
        name: "gggg",
        command: "",
        dir: "",
        auto_start: true,
        process_num: 1,
        start_retries:5,
    },
    status: "running"
}];

var _lastLogContent = "";
var _lastLogLineNum = 0;
var _logDiv = null;
var _lineNumDiv = null;
var vm = new Vue({
    el: "#app",
    data: {
        isConnectionAlive: true,
        log: {
            content: '',
            log_process: "",
            follow: true,
            line_count: 0
        },
        programs: [],
        edit: {
            program: null
        },
        host: host,
        current_user: current_user,
        is_admin: is_admin
    },
    methods: {
        addNewProgram: function () {
            console.log("Add");
            var form = $("#form_new_program");
            form.submit(function (e) {
                e.preventDefault();
                $("#new_program").modal('hide');
                return false;
            });
        },
        showEditProgram: function (p) {
            // v-on:click
            // 参数从哪儿来? 直接是一个javascript对象?
            //
            this.edit.program = Object.assign({}, p);
            $("#program_edit").modal('show');

        },
        editProgram: function () {
            console.log(this.edit.program);
            var p = this.edit.program;

            p.start_retries = parseInt(p.start_retries);
            p.process_num = parseInt(p.process_num);
            p.stop_timeout = parseInt(p.stop_timeout);

            $.ajax({
                url: "/" + vm.host + "/api/programs/" + p.name,
                method: "PUT",
                data: JSON.stringify(p),
                success: function (data) {
                    if (data.status === 0) {
                        alertify.success("新程序添加成功");
                        $("#program_edit").modal('hide');
                    } else {
                        alertify.error(data.error);
                    }
                },
                error: function (err) {
                    alertify.error(err.responseText);
                    console.log(err.responseText);
                }
            });
        },
        refresh: function () {
            // 如何防止重复无效请求呢?
            // 1s内请求最多一次
            _refreshRequestNum++;
            refreshRequest();
        }

        ,
        reload: function () {
            $.ajax({
                url: "/" + vm.host + "/api/reload",
                method: "POST",
                success: function (data) {
                    if (data.status == 0) {
                        alert("reload success");
                    } else {
                        alert(data.value);
                    }
                }
            });
        }
        ,
        test: function () {
            console.log("test");
        }
        ,
        cmdStart: function (name) {
            console.log(name);
            $.ajax({
                url: "/" + vm.host + "/api/programs/" + name + "/start",
                method: 'post',
                success: function (data) {
                    console.log(data);
                }
            })
        }
        ,
        cmdStop: function (name) {
            $.ajax({
                url: "/" + vm.host + "/api/programs/" + name + "/stop",
                method: 'post',
                success: function (data) {
                    console.log(data);
                }
            })
        }
        ,
        cmdTail: function (name) {
            var that = this;
            if (W.wsLog) {
                W.wsLog.close();
                W.wsLog = null;
            }

            _logDiv = $(".realtime-log");
            _lineNumDiv = $("#line_count")[0];

            clearLogsWithTitle("程序" + name + "所有进程");
            // 如何查看日志呢?
            W.wsLog = newWebsocket("/" + vm.host + "/ws/logs/" + name, {
                onopen: function (evt) {
                    clearLogsWithTitle("程序" + name + "所有进程");
                },
                onmessage: processLogEvent
            });
            // 默认follow为true
            this.log.follow = true;

            $("#modal_tailf").modal({
                show: true,
                keyboard: true
            })
        }
        ,
        cmdDelete: function (name) {
            if (!confirm("Confirm delete \"" + name + "\"")) {
                return
            }
            $.ajax({
                url: "/" + vm.host + "/api/programs/" + name,
                method: 'delete',
                success: function (data) {
                    console.log(data);
                }
            })
        }
        ,
        canStop: function (status) {
            switch (status) {
                case "running":
                case "retry wait":
                    return true;
            }
        }
    }
});

var _refreshRequestNum = 0;
var _refreshRequestTimeout = null;
function refreshRequest() {
    if (_refreshRequestNum > 0 && _refreshRequestTimeout == null) {
        _refreshRequestTimeout = setTimeout(function () {
            _refreshRequestNum = 0;
            $.ajax({
                url: "/" + vm.host + "/api/programs",
                success: function (data) {
                    vm.programs = data;
                    Vue.nextTick(function () {
                        $('[data-toggle="tooltip"]').tooltip()
                    })
                },
                complete: function () {
                    _refreshRequestTimeout = null;
                    if (_refreshRequestNum > 0) {
                        refreshRequest();
                    }
                }
            });
        }, 1);
    }
}

function clearLogsWithTitle(title) {
    _lastLogLineNum = 0;
    _lastLogContent = "";
    vm.log.log_process = title;
    _logDiv.html("");
}

var URL_PATTERN = "Successfully uploaded to ";
var PHP_ERROR = "handleError";
var PHP_ERROR_HINT = "<span style='color:#ff01e3;'>handleError</span>";
function processLogEvent(evt) {
    // 不follow时，一般是希望停下来查看日志
    if (!vm.log.follow) {
        return;
    }
    var content = _lastLogContent + evt.data.replace(/\033\[[0-9;]*m/g, "");
    var lines = content.split(/\n/);

    var totalLength = lines.length;
    if (content.endsWith('\n')) {
        _lastLogContent = "";
    } else {
        _lastLogContent = lines[totalLength - 1];
        totalLength = totalLength - 1;
    }

    var line_count = _lastLogLineNum;
    for (var i = 0; i < totalLength; i++) {
        var line = lines[i].trim();

        if (line.length > 0 && !line.endsWith(".ts")) {
            line_count++;

            // 添加视频相关的链接
            var index = line.indexOf(URL_PATTERN);
            if (index != -1 && !line.endsWith("merged.m3u8")) {
                var url = decodeURIComponent(line.substring(index + URL_PATTERN.length));

                var matches = url.match(/recordings\/(\d+)/i);
                if (matches && matches.length == 2) {
                    line = line.substring(0, index + URL_PATTERN.length) + "<a href='https://back-service.ushow.media/recording/record/" + matches[1] + "' target=_blank>" + url + "</a>";
                }
            }

            // 添加错误处理
            index = line.indexOf(PHP_ERROR);
            if (index != -1) {
                line = line.substring(0, index) + PHP_ERROR_HINT + line.substring(index + PHP_ERROR.length);
            }


            if (line_count > 1000) {
                var child = _logDiv[0].childNodes[0];
                _logDiv[0].removeChild(child);
                child.innerHTML = line;
                _logDiv[0].appendChild(child);
            } else {
                _logDiv.append("<p>" + line + "</p>");
            }
        }
    }
    _lastLogLineNum = line_count;
    _lineNumDiv.innerHTML = "Line: " + _lastLogLineNum;

    if (vm.log.follow) {
        var pre = _logDiv[0];
        setTimeout(function () {
            pre.scrollTop = pre.scrollHeight - pre.clientHeight;
        }, 1);
    }
}

// Vue的filter的定义: 和Django中的filter类似
Vue.filter('fromNow', function (value) {
    return moment(value).fromNow();
});

Vue.filter('formatBytes', function (value) {
    var bytes = parseFloat(value);
    if (bytes < 0) return "-";
    else if (bytes < 1024) return bytes + " B";
    else if (bytes < 1048576) return (bytes / 1024).toFixed(0) + " KB";
    else if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + " MB";
    else return (bytes / 1073741824).toFixed(1) + " GB";
});

//
// 使用场合: p | colorStatus
//
Vue.filter('colorStatus', function (value) {
    var makeColorText = function (text, color) {
        return "<span class='status' style='background-color:" + color + "'>" + text + "</span>";
    };
    switch (value.status) {
        case "stopping":
            return makeColorText(value.process_num + " " + value.status, "#996633");
        case "running":
            var running = value.running_num + "/" + value.process_num;
            return makeColorText(running + " " + value.status, "green");
        case "fatal":
            return makeColorText(value.process_num + " " + value.status, "red");
        default:
            return makeColorText(value.process_num + " " + value.status, "gray");
    }
});

Vue.directive('disable', function (value) {
    // 直接修改 model 的状态
    this.el.disabled = !!value
});

$(function () {
    // 更新programs的状态
    vm.refresh();

    // 强制刷新时，更新programs的状态
    $("#form_new_program").submit(function (e) {
        var url = "/" + vm.host + "/api/programs",
            data = $(this).serialize();
        $.ajax({
            type: "POST",
            url: url,
            data: data,
            success: function (data) {
                if (data.status === 0) {
                    alertify.success("新程序添加成功");
                    $("#new_program").modal('hide');
                } else {
                    alertify.error(data.error);
                }
            },
            error: function (err) {
                alertify.error(err.responseText);
                console.log(err.responseText);
            }
        });
        e.preventDefault()
    });

    function newEventWatcher() {

        var heartbeat_msg = '--heartbeat--', heartbeat_interval = null, missed_heartbeats = 0;

        function on_open() {
            if (heartbeat_interval === null) {
                missed_heartbeats = 0;
                heartbeat_interval = setInterval(function () {
                    try {
                        missed_heartbeats++;
                        if (missed_heartbeats >= 3 || W.events == null) {
                            throw new Error("Too many missed heartbeats.");
                        }
                        W.events.send(heartbeat_msg);
                    } catch (e) {
                        clearInterval(heartbeat_interval);
                        heartbeat_interval = null;
                        console.warn("Closing connection. Reason: " + e.message);
                        if (W.events != null) {
                            W.events.close();
                        }
                    }
                }, 5000);
            }
        }

        W.events = newWebsocket("/" + vm.host + "/ws/events", {
            onopen: function (evt) {
                vm.isConnectionAlive = true;
                on_open();
            },
            onmessage: function (evt) {
                // 收到消息之后，就更新状态
                if (evt.data == heartbeat_msg) {
                    missed_heartbeats = 0;
                } else {
                    console.log("response:" + evt.data);
                    vm.refresh();
                }
            },
            onclose: function (evt) {
                W.events = null;
                vm.isConnectionAlive = false;
                console.log("Reconnect after 3s");
                setTimeout(newEventWatcher, 3000)
            }
        });
    }

    newEventWatcher();

    // cancel follow log if people want to see the original data
    $(".realtime-log").bind('mousewheel', function (evt) {
        if (evt.originalEvent.wheelDelta >= 0) {
            vm.log.follow = false;
        }
    });
    $('#modal_tailf').on('hidden.bs.modal', function () {

        if (W.wsLog) {
            W.wsLog.close();
            W.wsLog = null;
        }
    })
});
