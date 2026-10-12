const API_BASE = window.location.pathname.replace(/\/$/, '') + '/api';
let token = localStorage.getItem('hs_token') || '';

// 初始化
if (!token) {
    document.getElementById('login-modal').style.display = 'flex';
} else {
    document.getElementById('login-modal').style.display = 'none';
    document.getElementById('app').style.display = 'block';
    fetchStatus();
    fetchNodes();
    setInterval(fetchStatus, 3000); // 每3秒刷新状态
}

function login() {
    token = document.getElementById('pwd-input').value;
    localStorage.setItem('hs_token', token);
    location.reload();
}

async function apiCall(endpoint, options = {}) {
    options.headers = { ...options.headers, 'Authorization': `Bearer ${token}` };
    const res = await fetch(API_BASE + endpoint, options);
    if (res.status === 401) {
        localStorage.removeItem('hs_token');
        location.reload();
    }
    return res.json();
}

async function fetchStatus() {
    try {
        const data = await apiCall('/status');
        const s = data.state;
        const c = data.config;
        
        const btnM = document.getElementById('btn-master');
        btnM.textContent = s.master_switch ? '🟢 SOCKS5: 工作中' : '🔴 SOCKS5: 已关闭';
        btnM.className = s.master_switch ? 'active' : 'danger';

        const btnR = document.getElementById('btn-route');
        btnR.textContent = s.routing_mode === 'route' ? '✈️ 模式: 走隧道' : '🏠 模式: 宿主机直连';
        btnR.className = s.routing_mode === 'route' ? 'active' : '';

        document.getElementById('ui-port').textContent = c.socks_port + (c.socks_auth ? ' (需密码)' : '');
        document.getElementById('ui-ip').textContent = s.current_node_ip || '未分配';
        document.getElementById('ui-mode').textContent = s.node_select_mode === 'auto' ? '🤖 自动探测最低延迟' : '✋ 手动固定';
        
        const dUI = document.getElementById('ui-delay');
        if (s.routing_mode === 'direct') {
            dUI.textContent = '直连无测速'; dUI.className = '';
        } else {
            dUI.textContent = data.delay > 0 ? `${data.delay} ms` : '超时或断开';
            dUI.className = data.delay > 0 && data.delay < 1000 ? 'delay' : 'delay bad';
        }
    } catch (e) { console.error(e); }
}
async function fetchNodes() {
    try {
        const nodes = await apiCall('/nodes');
        const sData = await apiCall('/status');
        const currentIP = sData.state.current_node_ip;

        const list = document.getElementById('node-list');
        list.innerHTML = '';

        if (nodes.length === 0) {
            list.innerHTML = '<p>尚未拉取到可用节点，请等待后台 Action 同步...</p>';
            return;
        }

        nodes.forEach(n => {
            const div = document.createElement('div');
            div.className = `node-item ${n.ip === currentIP ? 'current' : ''}`;
            
            div.innerHTML = `
                <div class="node-info">
                    <span class="country-badge">${n.country}</span>
                    <b>${n.ip}</b>
                    <span style="font-size:12px; color:#aaa; margin-left:10px;">(宣称 Ping: ${n.ping}ms | 速度: ${n.speed})</span>
                </div>
                <div class="node-actions">
                    <span id="delay-${n.ip.replace(/\./g, '-')}" style="font-size:12px; color:#ff9800; margin-right:10px;"></span>
                    <button onclick="testNode('${n.ip}')">⚡ 测真连</button>
                    <button class="${n.ip === currentIP ? 'active' : ''}" onclick="selectNode('${n.ip}')">📍 使用</button>
                </div>
            `;
            list.appendChild(div);
        });
    } catch (e) { console.error(e); }
}
async function doAction(act) {
    await apiCall('/action', {
        method: 'POST',
        body: JSON.stringify({ action: act })
    });
    fetchStatus();
    if(act === 'set_auto') {
        alert('已开启自动模式，后台将持续寻找最快节点！');
        fetchNodes();
    }
}
async function selectNode(ip) {
    await apiCall('/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'set_manual', ip: ip })
    });
    fetchStatus();
    fetchNodes();
}
async function testNode(ip) {
    const delayUI = document.getElementById(`delay-${ip.replace(/\./g, '-')}`);
    delayUI.textContent = '测试中...';
    try {
        const res = await apiCall('/test', {
            method: 'POST',
            body: JSON.stringify({ ip: ip })
        });
        if (res.ok && res.delay > 0) {
            delayUI.textContent = `${res.delay} ms`;
            delayUI.style.color = '#4caf50';
        } else {
            delayUI.textContent = '不通';
            delayUI.style.color = '#f44336';
        }
    } catch (e) {
        delayUI.textContent = '测速出错';
    }
}