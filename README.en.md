# NetGap

[简体中文](README.md) | [English](README.en.md)

NetGap checks whether the public IP address seen through your proxy remains in an expected network region.

It provides a command line application for scripts and terminal use, and a desktop GUI for configuration, monitoring status, logs, and route actions.

## Disclaimer

By continuing to read the project materials or using the software, you agree to the [NOVASEC disclaimer](https://mp.weixin.qq.com/s/iRWRVxkYu7Fx5unxA34I7g).

## Applications

- [Command line app](cmd/netgap/README.md): periodically queries the proxy egress IP, matches it against configured regions, and can optionally remove the default route when a match is detected.
- [Desktop GUI](cmd/netgapgui/README.md): configures regions, intervals, and IP lookup APIs; displays monitoring logs; and provides route inspection and manual route actions.

Both applications detect system proxy settings, query enabled IP APIs in order, and use bundled local IPv4/IPv6 databases to look up the resulting IP location. Custom databases can be placed in an `assets` directory next to the executable.

## How it works

1. NetGap resolves the proxy settings and bypass rules that apply to each API URL. Windows and macOS support fixed proxies and PAC/WPAD. Linux supports fixed proxies and bypass rules; PAC/WPAD is reported as a limited mode and automatic route removal is paused.
2. It queries enabled IP APIs until one returns a valid IP address. Failed APIs are retried and temporarily cooled down after repeated failures.
3. It looks up the IP location in the bundled IPv4/IPv6 databases and matches the result against configured region keywords.
4. Depending on the application and permissions, a region match can trigger a warning or save and remove the default route. Route changes require administrator privileges on Windows and effective root privileges on Linux/macOS.

Removing a default route interrupts network connectivity. Route removal is not a complete network isolation mechanism; more specific routes, directly connected networks, VPN routes, or routes recreated by the operating system may remain usable. Recovery data is saved before removal, but recovery should be performed from the same account and data directory.

## Supported platforms

| Platform | Release builds | Notes |
| --- | --- | --- |
| Windows | amd64 CLI and GUI | System fixed proxy, PAC/WPAD, and native route operations. |
| Linux | amd64 CLI and GUI | Ubuntu 22.04 and 24.04 are built in CI. GUI builds require GTK and WebKitGTK development packages. PAC/WPAD is not evaluated; automatic route removal is paused in this mode. |
| macOS | arm64 CLI and GUI | Release currently targets Apple Silicon only. Native proxy and route support requires Apple SDKs and CGO. Intel Macs do not receive a release build or automatic updates. |
| Other platforms | Not currently released | Full default-route protection is refused on unsupported platforms. |

See [macOS support notes](docs/macos-support.md) and [Linux proxy support notes](docs/linux-proxy-support.md) for platform details.

## Downloads

Download prebuilt applications from [GitHub Releases](https://github.com/winezer0/netgap/releases):

- Windows: `netgapgui-windows-amd64.zip`
- Linux: `netgapgui-linux-amd64.tar.gz`
- macOS Apple Silicon: `netgapgui-macos-arm64.zip`

Each release includes a `SHA256SUMS` file. Linux 24.04 builds use the WebKit 4.1 stack; use the matching Ubuntu release and dependencies.

## Build from source

The repository currently requires Go 1.26.0 as declared in [go.mod](go.mod). GUI builds also require Node.js 22 and Wails v3. Linux GUI builds need the platform GTK/WebKitGTK development packages. macOS native builds require Xcode Command Line Tools and CGO.

From the repository root:

```sh
go build -o netgap ./cmd/netgap
cd cmd/netgapgui
node build.mjs
```

For Linux release archives, build with the `dezip_full` tag. Ubuntu 24.04 GUI builds also use `webkit2_41`:

```sh
node build.mjs -tags dezip_full
# Ubuntu 24.04
node build.mjs -tags webkit2_41,dezip_full
```

On macOS, build the release architecture with:

```sh
CGO_ENABLED=1 node build.mjs -platform darwin/arm64
```

See the [CLI guide](cmd/netgap/README.md) and [GUI guide](cmd/netgapgui/README.md) for usage and configuration.

## Permissions and recovery

IP lookup, region matching, logs, proxy inspection, and route inspection are read-only and normally do not require administrator permissions. Changing the system default route does.

Before removing a route, NetGap saves route recovery data. To restore routes, use the same account and data directory that performed the removal. The CLI provides a recovery precheck with `--recovery-check --recovery-dir <snapshot-directory>`; the check does not modify routes and does not replace a real recovery test.

## Validation status

CI builds and automated checks cover Windows amd64, Linux amd64, and macOS arm64. These checks do not constitute full interactive desktop acceptance. Real route removal and recovery should only be tested in an isolated environment with a verified recovery path.

See the [verified scenarios](docs/已验证场景记录.md), [remaining scenarios](docs/剩余未验证场景记录.md), and [desktop validation guide](docs/native-desktop-validation.md) for current evidence and acceptance requirements.

