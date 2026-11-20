# 移动端 CI 与发布

工作流沿用 `.github/workflows/ci.yml`。Android 使用 Ubuntu 24.04，iOS 使用 macOS 15 Apple Silicon，与桌面测试并行执行。相比把 Android 串行塞进 macOS 打包步骤，这样能减少 Mac 队列和总体等待，并单独定位失败的平台。

## 检查范围

`.github/scripts/test-platforms.sh` 根据差异选择是否运行移动端检查。Go 源码、依赖、资源、原生模块、配置和构建流程变更会触发；纯文档改动不会。版本标签和手动触发总是检查。

- 共享逻辑由现有 `go test ./...` 覆盖，包括 viewport、A2UI、配置和图片原始文件缓存。
- 移动端任务调用同版本 MyGo CLI，实际编译 Go、Swift/Kotlin 原生模块和资源，并链接系统 SDK。
- CI 只做编译和打包，不下载模拟器系统镜像，不安装或启动应用，不截图，也不发起模型请求。键盘、系统代理与 PAC、系统分享、权限和安全区由本地专项验收覆盖。

Android 日常检查和版本标签统一构建 arm64（`arm64-v8a`）。Eino 间接依赖 Sonic，后者不支持 32 位，因此不使用 MyGo 默认包含 `armeabi-v7a` 的全 ABI 构建；当前也不发布 x86_64 APK。NDK 固定为 28.2.13676358，JDK 17，Android SDK 36，缓存 Gradle 依赖。iOS 未配置签名时，使用 SDK 的 arm64 simulator 目标完成编译检查，不运行模拟器；配置签名后，发版额外构建真机 IPA。使用标准 `macos-15` Apple Silicon runner。

Android 仅发布一个 arm64 APK，不生成 universal 发布包。CI 默认设置 `MICROLIGHT_ANDROID_COMPRESS_LIBS=1` 压缩 APK 中的原生库，可显式设为 `0` 关闭；本地也可使用 `MICROLIGHT_ANDROID_COMPRESS_LIBS=1 bash scripts/build.sh -platform android/arm64`。设为 `0` 或不设置时保留 MyGo 默认的未压缩打包。MyGo v0.10.0 本身没有对应开关，项目通过临时、限定项目路径的 Gradle init script，在 AGP `finalizeDsl` 中设置 `useLegacyPackaging=true`，构建结束自动清理；不修改 SDK 源码或生成的 Gradle 文件。压缩模式安装时需解压原生库，以减少下载文件大小。MyGo v0.10.0 的构建命令仍会同时生成 AAB，但 CI 不收集或上传它；未来需要上架 Google Play 时再启用。

## 发布产物与签名

版本标签使用 Git 标签注入应用版本和包版本。只有桌面测试和移动端编译打包全部通过才发布 Release。

默认产物：

- `microlight-vX.Y.Z-ios-simulator-arm64.zip`：模拟器 `.app`，不能安装到 iPhone。
- `microlight-vX.Y.Z-android-arm64.apk`：使用固定发布密钥签名，后续版本保持同一签名身份。

正式签名仅在版本标签的构建步骤读取 GitHub Actions secrets：

| 平台 | Secrets | 用途 |
| --- | --- | --- |
| iOS | `IOS_CERTIFICATE_BASE64`、`IOS_CERTIFICATE_PASSWORD`、`IOS_PROFILE_BASE64` | 含私钥的 P12 证书、其密码和匹配应用标识的 provisioning profile |
| Android | `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEY_ALIAS`、`MYGO_ANDROID_KEYSTORE_PASSWORD`、`MYGO_ANDROID_KEY_PASSWORD` | 固定 APK 发布签名密钥和密码 |

iOS 配齐后，构建额外生成 `microlight-vX.Y.Z-ios-arm64.ipa`；安装范围由描述文件决定，App Store 类型 IPA 用于 TestFlight/App Store，不能任意侧载。Android 标签构建必须配齐签名材料；缺失任何一项都失败，不回退到调试签名。产物命名不添加 debug 或 release 后缀，构建后使用 apksigner 校验签名，并与 keystore 的证书 SHA-256 核对。普通分支编译检查仍可用调试签名，不上传到 Release。签名文件存临时目录，iOS 使用临时 keychain，步骤结束后清理；不会提交到仓库或作为 artifact 上传。iOS 没有签名材料时明确输出 notice，继续发布标注用途的模拟器产物。

这套流程不自动上传 App Store Connect 或 Google Play，也不绕过已有的 commit、push、tag 授权流程。

参考：[MyGo 移动端构建与签名](https://github.com/egoist/mygo/blob/v0.10.0/docs/mobile/building.md)、[GitHub runner 架构](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)。

### Android 密钥生成与配置

```sh
bash scripts/android/signing.sh init
gh auth login --hostname github.com --git-protocol ssh --skip-ssh-key --web
bash scripts/android/signing.sh upload
```

默认保存在 `~/.config/microlight/android-signing/`，目录权限 700、文件权限
600；可用 `MICROLIGHT_ANDROID_SIGNING_DIR` 指定位置。首次生成 RSA 3072 位
PKCS12 keystore、随机密码、alias 与公开证书。再次执行复用原密钥，不覆盖；
不完整的目录会报错，避免意外换签名。将整个目录另行安全备份，后续版本沿用
同一签名身份。脚本将四项内容通过 stdin 上传为仓库 Actions Secrets，
不把密码或私钥写入命令参数、Git 或发布附件。

CI 只在版本标签构建时读取密钥，在临时目录还原 keystore，构建结束清理。
现有调试签名安装与新的发布签名不同，不能直接覆盖安装；卸载旧包会删除
应用数据，应先导出需要保留的配置。后续发布版之间可以沿用相同签名更新。
