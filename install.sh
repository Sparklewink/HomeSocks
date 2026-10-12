#!/usr/bin/env bash
set -e

REPO_OWNER="Sparklewink"
REPO_NAME="HomeSocks"

# 默认参数
PANEL_PORT=8899
SOCKS_PORT=1080
PANEL_PATH="/$(tr -dc A-Za-z0-9 </dev/urandom | head -c 8)"
PASSWORD=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 12)
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/homesocks"

echo -e "\n=== 开始安装 HomeSocks ==="

if [[ $EUID -ne 0 ]]; then
   echo "错误：必须使用 root 权限运行此脚本" 1>&2
   exit 1
fi

if [[ ! -c /dev/net/tun ]]; then
    echo "错误：/dev/net/tun 不可用。若是 LXC 容器，请在母鸡放权 tun 设备。"
    exit 1
fi

echo "-> 检查并安装依赖 (iproute2, iptables, openvpn, curl)..."
if command -v apt-get >/dev/null; then
    apt-get update -y && apt-get install -y iproute2 iptables openvpn curl jq
elif command -v yum >/dev/null; then
    yum install -y iproute iptables openvpn curl jq
elif command -v apk >/dev/null; then
    apk add iproute2 iptables openvpn curl jq
else
    echo "未知包管理器，请手动安装 iproute2, iptables, openvpn, curl"
    exit 1
fi

ARCH=$(uname -m)
case $ARCH in
    x86_64) BIN_ARCH="amd64" ;;
    aarch64|arm64) BIN_ARCH="arm64" ;;
    *) echo "不支持的 CPU 架构: $ARCH"; exit 1 ;;
esac

echo "-> 从 GitHub 拉取最新版二进制 ($BIN_ARCH)..."
API_URL="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
DOWNLOAD_URL=$(curl -s $API_URL | jq -r ".assets[] | select(.name | contains(\"${BIN_ARCH}\")) | .browser_download_url" | head -n 1)

if [[ -z "$DOWNLOAD_URL" || "$DOWNLOAD_URL" == "null" ]]; then
    echo "错误：未能找到对应的 Release 预编译版本，请确认仓库存在对应架构的资产。"
    exit 1
fi

curl -L "$DOWNLOAD_URL" -o "${INSTALL_DIR}/homesocks"
chmod +x "${INSTALL_DIR}/homesocks"

echo "-> 生成配置文件到 ${CONFIG_DIR} ..."
mkdir -p "$CONFIG_DIR"
cat > "${CONFIG_DIR}/config.json" <<EOF
{
  "panel_port": $PANEL_PORT,
  "panel_path": "$PANEL_PATH",
  "panel_password": "$PASSWORD",
  "socks_port": $SOCKS_PORT,
  "socks_listen": "0.0.0.0",
  "socks_auth": {
    "enabled": true,
    "username": "homesocks",
    "password": "$PASSWORD"
  },
  "health_check_interval": 600,
  "node_sync": {
    "source_url": "https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/nodes.csv",
    "interval_hours": 24
  }
}
EOF

echo "-> 注册 Systemd 服务..."
cat > /etc/systemd/system/homesocks.service <<EOF
[Unit]
Description=HomeSocks SOCKS5 Gateway
After=network.target

[Service]
Type=simple
User=root
ExecStart=${INSTALL_DIR}/homesocks -c ${CONFIG_DIR}/config.json
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable homesocks
systemctl restart homesocks

echo -e "\n=== 安装完成 ==="
echo "服务已启动并加入开机自启。"
echo "----------------------------------------"
echo "🌐 面板地址: http://$(curl -s ifconfig.me):${PANEL_PORT}${PANEL_PATH}"
echo "🔑 面板密码: $PASSWORD"
echo "🔌 SOCKS5  : 端口 ${SOCKS_PORT} | 账号 homesocks | 密码 $PASSWORD"
echo "----------------------------------------"
echo "常用命令："
echo "  查看状态: systemctl status homesocks"
echo "  重启服务: systemctl restart homesocks"
echo "  查看日志: journalctl -u homesocks -f"
echo "配置文件路径: ${CONFIG_DIR}/config.json"