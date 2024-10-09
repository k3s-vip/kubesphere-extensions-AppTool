# 商店导入工具

## 概述

本工具用于把 helm repo 中的软件同步到应用商店中。注意, 您可以在 kubespehre 直接配置repo源使用, 参考文档

https://ask.kubesphere.io/forum/d/23922-kubesphere-411-ying-yong-shang-dian-pei-zhi-fang-fa

这个工具是把 repo 中的应用变成全局商店应用, 不是必须的操作。

## 使用方法

配置安装参数, 指定要同步的源

```bash
app-tool:
  repoUrl: "https://charts.kubesphere.io/stable"
```

## 注意事项

由于商店允许多次上传并生成随机名称的应用，本工具不会处理多次执行的场景。如果您多次执行，希望清理生成的资源，请手动执行

```
kubectl delete applications.application.kubesphere.io xxx
```
