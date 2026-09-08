# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

This is a fork of [tailscale/tailscale-android](https://github.com/tailscale/tailscale-android)
(`upstream` remote); `origin` is `Craeckie/tailscale-android`. Sync with
`git fetch upstream && git merge upstream/main`.

A hybrid app: the whole Tailscale backend is Go, compiled to an AAR with `gomobile bind`, wrapped by
a Kotlin/Jetpack Compose UI. The `Makefile` at the repo root drives both halves — Gradle alone is
not enough to build the app.

## Commands

```bash
make apk                # tailscale-debug.apk (builds the AAR first, then ./gradlew test assembleDebug)
make install            # adb install -r tailscale-debug.apk
make run                # install + adb shell am start -n com.tailscale.ipn/.MainActivity
make test               # Kotlin/JVM unit tests (cd android && ./gradlew test)
make go-test            # Go tests, excluding ./libtailscale (needs the NDK to link)
make fmt / make fmt-check   # ktfmt via ./gradlew ktfmtFormat / ktfmtCheck
make androidsdk         # install the pinned SDK/NDK/build-tools packages
make emulator           # create+start the tailscale-<arch> AVD
make env                # print resolved ANDROID_HOME / JAVA_HOME etc. when the build can't find them
make clean              # also drops the cached Go toolchain and tailscale.version
```

Single Kotlin test (the Makefile doesn't wrap this):
`cd android && ./gradlew testDebugUnitTest --tests "com.tailscale.ipn.IPNServiceTest"`.
Instrumented tests are under `android/src/androidTest/`, built via the custom `applicationTest`
build type (`testBuildType "applicationTest"` in `android/build.gradle`).

Releases: `make release` (phone/tablet AAB) and `make release-tv` (Android TV AAB, `-PPLATFORM=tv`).
Both need `JKS_PATH` and `JKS_PASSWORD` — **signing is a post-build `jarsigner` step in the
Makefile, there is no `signingConfigs` block in Gradle at all.** TV is not a product flavor either:
`isTV()` in `android/build.gradle` toggles the `leanbackRequired` manifest placeholder and the last
digit of `versionCode`.

Docker and Nix exist for reproducibility, not as requirements: `make docker-shell` /
`make docker-run-build` use `docker/DockerFile.amd64-build` (pinned JDK + NDK, persistent
`.android-docker`/`.gradle-docker` mounts so the debug keystore survives), and `nix develop` works
off `flake.nix`. `make android-integration-test` needs Docker *and* `/dev/kvm` — it runs an emulator
inside `docker/Dockerfile.android-integration`; bump `ANDROID_INTEGRATION_DOCKER_IMAGE` in the
Makefile when that image changes.

## The Go ↔ Kotlin bridge

The thing to understand before changing anything.

**Build side.** `make build-unstripped-aar` runs `gomobile bind -target android -androidapi 26` over
the `./libtailscale` package, then splits debug symbols out with the NDK's `llvm-objcopy` and repacks
a stripped `android/libs/libtailscale.aar`. Gradle consumes it as a flat-dir dependency
(`implementation ':libtailscale@aar'` + `flatDir { dirs 'libs' }`). `gomobile`/`gobind` are
`go install`ed on the fly into `android/build/go/bin`. Build tags come from `build-tags.sh` plus
`ts_omit_cachenetmap` (netmap caching is deliberately off on Android).

**Interface surface** — `libtailscale/interfaces.go` is the contract gomobile turns into Java classes:

- `Libtailscale.start(dataDir, directFileRoot, hwAttestation, appCtx)` is called exactly once, lazily,
  from `App.kt`, passing the Kotlin `App` singleton as the Go-defined `AppContext`. **Kotlin
  implements a Go interface**: Go calls back into it for logging, encrypted prefs, device info,
  syspolicy reads, hardware attestation, and `BindSocketToNetwork`.
- It returns a `libtailscale.Application`, whose main method is `CallLocalAPI` — an in-process,
  HTTP-shaped RPC mirroring tailscaled's localapi (method + endpoint + body). Kotlin wraps it in
  `ui/localapi/Client.kt`, which is the *only* sanctioned way for UI code to drive the backend.
- `WatchNotifications` streams `ipn.Notify` JSON from Go into Kotlin's `object Notifier`
  (`ui/notifier/Notifier.kt`), which fans it out into `StateFlow`s (`state`, `netmap`, `prefs`,
  `health`, `browseToURL`, `loginFinished`, incoming/outgoing files, …). That is the entire
  Go→Compose state pipeline.

**The VPN fd handoff.** `IPNService.kt` is a real Android `VpnService` that also implements the Go
`libtailscale.IPNService` interface. On start it calls `Libtailscale.requestVPN(this)`; Go then calls
back `NewBuilder()`, gets Kotlin's `VPNServiceBuilder` wrapping `android.net.VpnService.Builder`,
issues `AddRoute`/`AddAddress`/`AddDNSServer`/`SetMTU` calls that are forwarded 1:1, and finally
`Establish()` → `ParcelFileDescriptor.detach()` hands Go the **raw fd** that the Go netstack
(`wgengine/netstack`, `net/tsdial`) reads and writes packets on. Reconfiguration goes the other way
through `libtailscale/vpnfacade.go`, whose `VPNFacade` implements both `router.Router` and
`dns.OSConfigurator` and rebuilds the tunnel when config changes.

**Where `tailscale.com` comes from.** `go.mod` pins a plain pseudo-version
(`tailscale.com v1.103.0-pre.0.20260903171501-92ec102673bf`) fetched from the module proxy. There is
**no `replace` directive and no `go.work`** — this checkout is not wired to the sibling fork at
`/workspace/tailscale`. To build against that checkout you'd add `replace tailscale.com => ../tailscale`
yourself; keep it local and uncommitted, since CI's `go_mod_tidy.yml` won't tolerate it.
`make bumposs` / `make update-oss` are the sanctioned way to move the pin (they also sync
`go.toolchain.rev` from upstream `main`).

## Kotlin app architecture

Everything lives under `android/src/main/java/com/tailscale/ipn/`.

- `App.kt` — the `Application` subclass and singleton (`App.get()`), implements `libtailscale.AppContext`,
  owns notification channels and the `ViewModelStore`, and makes the one `Libtailscale.start(...)` call.
- `MainActivity.kt` — Compose entry point; one `NavHost` with string routes (`"settings"`,
  `"peerDetails/<id>"`, `"exitNodes"`, `"tailnetLock"`, `"loginWithCustomControl"`, …) behind a
  `DeepLinkNavigator`.
- `ui/model/` — `kotlinx.serialization` data classes mirroring the Go wire types (`Ipn.kt`,
  `NetMap.kt`, `TailCfg.kt`, `Health.kt`).
- `ui/viewModel/` and `ui/view/` are paired 1:1 by name. ViewModels collect `Notifier`'s `StateFlow`s
  and call `ui/localapi/Client.kt`; Views are pure renderers of their ViewModel's state.
- `mdm/` — `MDMSettings.kt` enumerates its own declared settings by Kotlin reflection and reads
  Android's `RestrictionsManager`. This is *separate from* Go's `syspolicy`: Go asks Kotlin for policy
  values through `AppContext.GetSyspolicy*` (`libtailscale/syspolicy_handler.go`), and Kotlin answers
  from the MDM layer. `MDMSettingsChangedReceiver` is registered in `App.onCreate()`.
- The VPN tunnel lifecycle (`IPNService`, `VPNServiceBuilder`, the `androidx.work` workers) is a
  second, largely independent path — it does not go through the ViewModel layer.

## Gotchas

- **`./tool/go`, not system Go.** It downloads `github.com/tailscale/go` at the rev in
  `go.toolchain.rev` into `~/.cache/tailscale-go`; `GOTOOLCHAIN=local` is forced. `TOOLCHAINDIR`
  overrides it (the F-Droid path), and `build-tags.sh` then emits `not_tailscale_go` instead of
  `tailscale_go`.
- **Pinned toolchain versions**: NDK `23.1.7779620` (the `-Wl,-z,max-page-size=16384` flag is
  NDK-23-specific; the Makefile auto-detects whatever is under `$ANDROID_HOME/ndk/*`, which on
  this machine is 28.2.13676358 — a mismatch worth checking first if the AAR link step fails),
  `androidApiLevel=36` / build-tools `36.0.0` / `minSdkVersion 26`
  (`android/gradle.properties`), Java 17 source+target and `jvmTarget = "17"`, Kotlin 1.9.22,
  Compose 1.5.10, AGP 8.13.0. The Docker image ships JDK 21.
- **Generated, never hand-edited or committed**: all of `android/libs/`, `libtailscale*.aar`,
  `libtailscale-sources.jar`, the `*.stripped`/`*.unstripped`/`*.debug` symbol files,
  `tailscale.version`, `android/local.properties`.
- **`tailscale.version` is regenerated** by `cmd/mkversion` from `go.mod`/`go.sum`/`go.toolchain.rev`/
  `.git/HEAD`, and feeds both the Go ldflags (`version-ldflags.sh`) and Gradle's `versionName`.
  Regenerate it (`make version`) after a commit that should be reflected in the stamped version.
- **`versionCode` is derived from wall-clock time** (`VERSION_CODE_BASE`), with the trailing digit
  distinguishing phone (0) from TV (1). Don't bump it by hand.
- **Pointing the app at a dev control server** has a first-class UI path: Settings → login with custom
  control URL (`ui/view/CustomLogin.kt`, route `"loginWithCustomControl"`). For scripted testing,
  `integration/androidvmtest/android_test.go` stands up a fake control server + DERP + STUN
  (`tstest/integration/testcontrol`) and drives an emulator over `adb`.
- DCO is required on commits: `git commit -s`.
