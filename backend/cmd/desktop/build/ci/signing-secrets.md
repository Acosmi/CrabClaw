# 桌面 Release 签名 Secret 说明

本文档定义当前桌面 release workflow `.github/workflows/desktop-release.yml` 预留的 Secret 名称。

## 1. Windows 安装器签名

- `DESKTOP_WINDOWS_CERT_BASE64`
  - base64 编码后的 PFX 证书
- `DESKTOP_WINDOWS_CERT_PASSWORD`
  - PFX 证书密码
- `DESKTOP_WINDOWS_SIGNTOOL_TIMESTAMP_URL`
  - 可选的时间戳服务器地址

当前行为：

- 若 Windows 证书 Secret 存在，workflow 会调用 `signtool.exe` 对 NSIS 安装器签名
- 若缺失，workflow 会输出 unsigned 状态文件

## 2. Release checksum 签名

- `DESKTOP_RELEASE_GPG_PRIVATE_KEY`
  - ASCII-armored 私钥文本，或 base64 编码后的私钥内容
- `DESKTOP_RELEASE_GPG_PASSPHRASE`
  - 私钥口令

当前行为：

- 若 GPG Secret 存在，workflow 会为 `SHA256SUMS` 生成 armored detached signature
- 若缺失，workflow 会输出 unsigned 状态文件
