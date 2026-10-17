const API_BASE = window.location.pathname.replace(/\/$/, '') + '/api';
let token = localStorage.getItem('hs_token') || '';
let globalNodes = [];

if (!token) {
    document.getElementById('login-modal').style.display = 'flex';
} else {
    document.getElementById('login-modal').style.display = 'none';
    document.getElementById('app').style.display = 'block';
    fetchStatus();
    fetchNodes();
    setInterval(fetchStatus, 3000);
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
        
        const modeUI = document.getElementById('ui-mode');
        if (s.node_select_mode === 'auto') {
            modeUI.innerHTML = '<span style="color:#2196f3;">🤖 自动寻优</span>';
        } else {
            modeUI.innerHTML = '<span style="color:#f44336;">🔒 手动锁定</span> (后台不再切换)';
        }

        const priInput = document.getElementById('priority-input');
        if (document.activeElement !== priInput && s.priority_countries) {
            priInput.value = s.priority_countries.join(', ');
        }
        
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
        globalNodes = await apiCall('/nodes');
        updateCountryFilter();
        renderNodes();
    } catch (e) { console.error(e); }
}

function updateCountryFilter() {
    const filter = document.getElementById('country-filter');
    const currentVal = filter.value;
    
    const countries = [...new Set(globalNodes.map(n => n.country))].sort();
    
    let html = '<option value="ALL">全部国家</option>';
    countries.forEach(c => {
        html += `<option value="${c}">${c}</option>`;
    });
    
    filter.innerHTML = html;
    if (countries.includes(currentVal) || currentVal === 'ALL') {
        filter.value = currentVal;
    }
}

async function renderNodes() {
    const sData = await apiCall('/status');
    const currentIP = sData.state.current_node_ip;
    const selectedCountry = document.getElementById('country-filter').value;

    const list = document.getElementById('node-list');
    list.innerHTML = '';

    if (globalNodes.length === 0) {
        list.innerHTML = '<p>尚未拉取到可用节点，请等待后台 Action 同步...</p>';
        return;
    }

    let displayNodes = globalNodes;
    if (selectedCountry !== 'ALL') {
        displayNodes = globalNodes.filter(n => n.country === selectedCountry);
    }

    displayNodes.forEach(n => {
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
                <button class="${n.ip === currentIP ? 'active' : ''}" onclick="selectNode('${n.ip}')">📍 锁定</button>
            </div>
        `;
        list.appendChild(div);
    });
}

async function doAction(act) {
    await apiCall('/action', {
        method: 'POST',
        body: JSON.stringify({ action: act })
    });
    fetchStatus();
    if(act === 'set_auto') {
        alert('已解除锁定，开启自动寻优！后台将按国家优先级切换最快节点。');
        fetchNodes();
    }
}

async function updatePriority() {
    const pStr = document.getElementById('priority-input').value;
    await apiCall('/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'update_priority', priority: pStr })
    });
    fetchStatus();
}

async function selectNode(ip) {
    await apiCall('/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'set_manual', ip: ip })
    });
    fetchStatus();
    renderNodes();
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