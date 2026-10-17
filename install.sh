#!/usr/bin/env bash
set -e
REPO_OWNER="Sparklewink"
REPO_NAME="HomeSocks"

PANEL_PORT=8899
SOCKS_PORT=1080
PANEL_PATH="/$(tr -dc A-Za-z0-9 </dev/urandom | head -c 8)"
PASSWORD=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 12)
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/homesocks"

GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
RESET="\033[0m"

echo -e "${CYAN}\n=== 开始安装 HomeSocks v2.0 ===${RESET}"

if [[ $EUID -ne 0 ]]; then
   echo -e "${YELLOW}错误：必须使用 root 权限运行此脚本${RESET}" 1>&2
   exit 1
fi
if [[ ! -c /dev/net/tun ]]; then
    echo -e "${YELLOW}错误：/dev/net/tun 不可用。若是 LXC 容器，请在母鸡放权 tun 设备。${RESET}"
    exit 1
fi

echo "-> 检查并安装依赖 (iproute2, openvpn, curl, jq)..."
if command -v apt-get >/dev/null; then
    apt-get update -y && apt-get install -y iproute2 iptables openvpn curl jq
elif command -v yum >/dev/null; then
    yum install -y iproute iptables openvpn curl jq
elif command -v apk >/dev/null; then
    apk add iproute2 iptables openvpn curl jq
else
    echo "未知包管理器，请手动安装 iproute2, iptables, openvpn, curl, jq"
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
    echo -e "${YELLOW}错误：未能找到预编译版本。如果是新仓库，请确保已发布 Release。${RESET}"
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

echo "-> 生成终端交互菜单 (hs)..."
cat > "${INSTALL_DIR}/hs" <<'EOF'
#!/usr/bin/env bash

# 颜色定义
GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
RESET="\033[0m"

show_menu() {
    echo -e "${CYAN}=== HomeSocks 管理菜单 ===${RESET}"
    echo -e " ${GREEN}1.${RESET} 查看运行状态"
    echo -e " ${GREEN}2.${RESET} 找回面板密码与地址"
    echo -e " ${GREEN}3.${RESET} 查看实时日志"
    echo -e " ${GREEN}4.${RESET} 重启服务"
    echo -e " ${GREEN}5.${RESET} 停止服务"
    echo -e " ${GREEN}6.${RESET} 启动服务"
    echo -e " ${YELLOW}9.${RESET} 彻底卸载"
    echo -e " ${GREEN}0.${RESET} 退出"
    echo -e "${CYAN}==========================${RESET}"
}

while true; do
    show_menu
    read -p "请输入数字选择功能 [0-9]: " choice
    echo ""
    case $choice in
        1)
            echo "=== HomeSocks 当前状态 ==="
            systemctl is-active --quiet homesocks && echo -e "服务进程: ${GREEN}运行中 🟢${RESET}" || echo -e "服务进程: \033[31m已停止 🔴${RESET}"
            if [ -f /etc/homesocks/state.json ]; then
                MODE=$(jq -r '.node_select_mode' /etc/homesocks/state.json)
                IP=$(jq -r '.current_node_ip' /etc/homesocks/state.json)
                PRI=$(jq -r '.priority_countries | join(", ")' /etc/homesocks/state.json)
                if [ "$MODE" = "auto" ]; then
                    echo -e "选路模式: ${CYAN}🤖 自动寻优${RESET} (优先: $PRI)"
                else
                    echo -e "选路模式: ${YELLOW}🔒 手动锁定${RESET}"
                fi
                echo "当前出口: $IP"
            fi
            echo "=========================="
            ;;
        2)
            echo "=== 面板与连接信息 ==="
            if [ -f /etc/homesocks/config.json ]; then
                P_PORT=$(jq -r '.panel_port' /etc/homesocks/config.json)
                P_PATH=$(jq -r '.panel_path' /etc/homesocks/config.json)
                P_PASS=$(jq -r '.panel_password' /etc/homesocks/config.json)
                S_PORT=$(jq -r '.socks_port' /etc/homesocks/config.json)
                PUB_IP=$(curl -s ifconfig.me || echo "你的公网IP")
                echo -e "🌐 面板地址: http://${PUB_IP}:${P_PORT}${P_PATH}"
                echo -e "🔑 面板密码: ${YELLOW}${P_PASS}${RESET}"
                echo -e "🔌 SOCKS5  : 端口 ${S_PORT} | 账号 homesocks | 密码 ${P_PASS}"
            else
                echo "未找到配置文件！"
            fi
            echo "======================"
            ;;
        3)
            echo "按 Ctrl+C 退出日志查看..."
            journalctl -u homesocks -f
            ;;
        4)
            systemctl restart homesocks
            echo -e "${GREEN}服务已重启。${RESET}"
            ;;
        5)
            systemctl stop homesocks
            echo -e "${YELLOW}服务已停止。${RESET}"
            ;;
        6)
            systemctl start homesocks
            echo -e "${GREEN}服务已启动。${RESET}"
            ;;
        9)
            read -p "⚠️ 确定要彻底卸载 HomeSocks 及其所有配置吗？(y/N): " confirm
            if [[ "$confirm" =~ ^[Yy]$ ]]; then
                homesocks-uninstall
                exit 0
            fi
            ;;
        0)
            exit 0
            ;;
        *)
            echo -e "${YELLOW}无效的选择，请重新输入。${RESET}"
            ;;
    esac
    echo ""
done
EOF
chmod +x "${INSTALL_DIR}/hs"

echo "-> 生成卸载工具 (homesocks-uninstall)..."
cat > "${INSTALL_DIR}/homesocks-uninstall" <<'EOF'
#!/usr/bin/env bash
echo "正在彻底卸载 HomeSocks..."
systemctl stop homesocks 2>/dev/null
systemctl disable homesocks 2>/dev/null
rm -f /etc/systemd/system/homesocks.service
systemctl daemon-reload
rm -rf /usr/local/bin/homesocks /usr/local/bin/hs /usr/local/bin/homesocks-uninstall
rm -rf /etc/homesocks
# 清理网络命名空间残留
echo "清理遗留的 netns 虚拟网络..."
ip netns list | grep -E '^hs-' | awk '{print $1}' | xargs -I {} ip netns del {} 2>/dev/null
echo "卸载完毕。系统已恢复干净状态。"
EOF
chmod +x "${INSTALL_DIR}/homesocks-uninstall"

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

PUBLIC_IP=$(curl -s ifconfig.me || echo "你的公网IP")

echo -e "\n${GREEN}====================================================${RESET}"
echo -e "${GREEN}             HomeSocks v2.0 安装成功! 🚀             ${RESET}"
echo -e "${GREEN}====================================================${RESET}"
echo -e " 🌐 ${CYAN}控制面板地址${RESET} : http://${PUBLIC_IP}:${PANEL_PORT}${PANEL_PATH}"
echo -e " 🔑 ${CYAN}面板登录口令${RESET} : ${YELLOW}${PASSWORD}${RESET}"
echo -e " 🔌 ${CYAN}SOCKS5 代理${RESET}  : 端口 ${YELLOW}${SOCKS_PORT}${RESET} | 账号: ${YELLOW}homesocks${RESET} | 密码: ${YELLOW}${PASSWORD}${RESET}"
echo -e "${GREEN}----------------------------------------------------${RESET}"
echo -e " 💡 ${CYAN}终端管理快捷键${RESET}:"
echo -e "    在命令行随时输入 ${YELLOW}hs${RESET} 即可唤出交互式管理菜单！"
echo -e "${GREEN}====================================================${RESET}\n"