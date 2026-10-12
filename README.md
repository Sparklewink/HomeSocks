# HomeSocks

「纯 SOCKS5 家宽出口」工具。基于 Linux Network Namespace (`netns`) 隔离 OpenVPN 进程，提供一个不污染宿主机全局路由、可随开随用、带可视化 Web 客户端的纯 SOCKS5 代理出口。

## 产品特性
* **纯净出口**：内建 Web 面板控制，对外提供单一标准 SOCKS5 接口。
* **物理隔离**：基于 `netns` 隔离隧道网络，本机其他应用完全不受影响。
* **自动/手动测速**：一键切换直连与隧道路由，面板内嵌真连接轻量级测速。
* **自动节点库**：利用 GitHub Action 每天自动从 VPN Gate 拉取并更新候选家宽节点池。
* **一键部署**：支持 Linux 宿主机直接一键安装或 Docker 容器化部署。

## 部署方法

### 方法一：宿主机一键安装（推荐 Ubuntu / Debian）
执行以下命令即可自动检测环境、安装依赖并注册 systemd 开机自启：
```bash
bash <(curl -fsSL [https://raw.githubusercontent.com/](https://raw.githubusercontent.com/)Sparklewink/HomeSocks/main/install.sh)