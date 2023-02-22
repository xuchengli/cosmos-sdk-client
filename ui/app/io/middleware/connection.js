'use strict';

const { spawn } = require('child_process');

module.exports = (app) => {
    return async (ctx, next) => {
        const top = spawn(`top -pid $(ps -ef | grep -v grep | grep simd | awk '{print $2}') -stats cpu`, {
            shell: true,
        });
        top.stdout.on('data', data => {
            const cpu = data.toString().split('\n')[12];

            // console.log('cpu使用率: ', cpu);

            ctx.socket.emit('res', cpu);
        });
        await next();
        // execute when disconnect.
        console.log('disconnection!');
    };
};
