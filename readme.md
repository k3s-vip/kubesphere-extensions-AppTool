# 商店导入工具

## 概述

本工具用于把helm repo中的软件同步到应用商店中。注意, 您可以在 kubespehre 直接配置repo源使用, 参考文档

https://ask.kubesphere.io/forum/d/23922-kubesphere-411-ying-yong-shang-dian-pei-zhi-fang-fa

这个工具是把repo中的应用变成全局商店应用, 不是必须的操作。

## 前提条件

- 可访问的 Kubernetes 集群，并配置好 `~/.kube/config` 文件
- 安装应用商店管理扩展

## 使用方法

### 命令行参数

- `--server`：kubespehre的服务器 URL（必填）
- `--username`：平台管理员级别的用户（必填）
- `--password`：密码（必填）
- `--repo`：Helm repo的 URL（必填）

### 示例

```bash
./app-tool  --server=http://192.168.50.87:30880 --username=admin --password=P@88w0rd --repo=https://charts.kubesphere.io/stable
```

## 注意事项

### 多次执行的场景

由于商店允许多次上传并生成随机名称的应用，本工具不会处理多次执行的场景。如果您多次执行，希望清理生成的资源，请手动执行
```
kubectl delete applications.application.kubesphere.io xxx
```
