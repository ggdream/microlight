# 移动端 CI 与发布

工作流沿用 `.github/workflows/ci.yml`。Android 使用 Ubuntu 24.04，iOS 使用 macOS 15 Apple Silicon，与桌面测试并行执行。相比把 Android 串行塞进 macOS 打包步骤，这样能减少 Mac 队列和总体等待，并单独定位失败的平台。

## 检查范围

`.github/scripts/test-platforms.sh` 根据差异选择是否运行移动端检查。Go 源码、依赖、资源、原生模块、配置和构建流程变更会触发；纯文档改动不会。版本标签和手动触发总是检查。

- 共享逻辑由现有 `go test ./...` 覆盖，包括 viewport、A2UI、配置和图片原始文件缓存。
- 移动端任务调用同版本 MyGo CLI，实际编译 Go、Swift/Kotlin 原生模块和资源，并链接系统 SDK。
- CI 只做编译和打包，不下载模拟器系统镜像，不安装或启动应用，不截图，也不发起模型请求。键盘、系统代理与 PAC、系统分享、权限和安全区由本地专项验收覆盖。

Android 非发布构建只编译 arm64，缩短反馈时间；版本标签编译 MyGo 默认支持的全部 Android ABI。NDK 固定为 28.2.13676358，JDK 17，Android SDK 36，缓存 Gradle 依赖。iOS 未配置签名时，使用 SDK 的 arm64 simulator 目标完成编译检查，不运行模拟器；配置签名后，发版额外构建真机 IPA。使用标准 `macos-15` Apple Silicon runner。

Android 仅发布 APK：发版时使用包含所有支持 ABI 的 universal APK，不拆分架构。MyGo v0.10.0 的构建命令仍会同时生成 AAB，但 CI 不收集或上传它；未来需要上架 Google Play 时再启用。

## 发布产物与签名

版本标签使用 Git 标签注入应用版本和包版本。只有桌面测试和移动端编译打包全部通过才发布 Release。

默认产物：

- `microlight-vX.Y.Z-ios-simulator-arm64.zip`：模拟器 `.app`，不能安装到 iPhone。
- `microlight-vX.Y.Z-android-debug.apk`：MyGo 默认调试签名，可直接安装试用；不同 runner 的调试签名也不能保证覆盖安装兼容。

正式签名仅在版本标签的构建步骤读取 GitHub Actions secrets：

| 平台 | Secrets | 用途 |
| --- | --- | --- |
| iOS | `IOS_CERTIFICATE_BASE64`、`IOS_CERTIFICATE_PASSWORD`、`IOS_PROFILE_BASE64` | 含私钥的 P12 证书、其密码和匹配应用标识的 provisioning profile |
| Android | `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEY_ALIAS`、`MYGO_ANDROID_KEYSTORE_PASSWORD`、`MYGO_ANDROID_KEY_PASSWORD` | 固定上传密钥和密码 |

iOS 配齐后，构建额外生成 `microlight-vX.Y.Z-ios-arm64.ipa`；安装范围由描述文件决定，App Store 类型 IPA 用于 TestFlight/App Store，不能任意侧载。Android 配齐后，产物标为 `android-release.apk`，使用稳定上传签名。只配置部分签名材料时构建失败，不静默降级。签名文件存临时目录，iOS 使用临时 keychain，步骤结束后清理；不会提交到仓库或作为 artifact 上传。没有签名材料时明确输出 notice，继续发布标注用途的测试产物。

这套流程不自动上传 App Store Connect 或 Google Play，也不绕过已有的 commit、push、tag 授权流程。

参考：[MyGo 移动端构建与签名](https://github.com/egoist/mygo/blob/v0.10.0/docs/mobile/building.md)、[GitHub runner 架构](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)。
