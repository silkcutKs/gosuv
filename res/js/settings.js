/* javascript */
var vm = new Vue({
    el: '#app',
    data: {
        name: name,
        index: index,
        pid: '-',
        childPids: [],
    }
});

var maxDataCount = 30;
var url = "/" + host + '/ws/perfs/' + name;
if (vm.index >= 0) {
    url = url + "/" + vm.index;
}
var ws = newWebsocket(url, {
    onopen: function (evt) {
        console.log(evt);
    },
    onmessage: function (evt) {
        var data = JSON.parse(evt.data);
        vm.pid = data.pid;
        vm.childPids = data.pids;
        console.log("pid", data.pid, data); //evt.data.pid);
        if (memData && data.rss) {
            memData.push({
                value: [new Date(), data.rss],
            })
            if (memData.length > maxDataCount) {
                memData.shift();
            }
            chartMem.setOption({
                series: [{
                    data: memData,
                }]
            });
        }
        if (cpuData && data.pcpu !== undefined) {
            cpuData.push({
                value: [new Date(), data.pcpu],
            })
            if (cpuData.length > maxDataCount) {
                cpuData.shift();
            }
            chartCpu.setOption({
                series: [{
                    data: cpuData,
                }]
            })
        }
    }
})
