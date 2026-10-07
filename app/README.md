# HomeProxy Android / Windows agent

## Windows GUI (wave 2)

Windows runner is `homeproxy-gui.exe`; Go `homeproxy.exe` stays CLI and
no-argument launcher. GUI never starts a second tunnel. It controls
`HomeProxyAgent` through native `homeproxy/windows` method channel and
`\\.\pipe\HomeProxyAgent.v1`. Backend contract: [WINDOWS_AGENT.md](../docs/WINDOWS_AGENT.md).

Requirements: Flutter stable, Visual Studio C++ desktop workload, Windows SDK,
Go matching root `go.mod`. From repository root:

```powershell
cd app
flutter analyze
flutter test
flutter build windows --release
cd ..
go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy
# Copy ENTIRE Flutter release directory, including DLLs and data/.
$package = 'bin/windows-package'
New-Item -ItemType Directory -Force $package | Out-Null
Copy-Item 'app/build/windows/x64/runner/Release/*' $package -Recurse -Force
Copy-Item 'bin/homeproxy.exe' "$package/homeproxy.exe" -Force
& "$package/homeproxy.exe"
```

Distribute this entire directory. Installer copies packaged Go helper to
ProgramFiles as `HomeProxy\homeproxy-service.exe`; no separate Flutter service
binary or GUI renaming. Never ship only GUI exe. Build/copy commands do not
install/start service. Do not run development package elevated.

GUI reports missing, stopped, transitioning or running SCM service. Install,
uninstall, start and stop each require consent followed by UAC. Original GUI
process user SID is captured before `ShellExecuteExW runas`; elevated helper
receives only fixed service command and SID, never token. Helper exit is awaited
(120-second bound); cancellation/nonzero exit/timeout are errors. Timed-out
helper is not killed and may finish: inspect SCM before retrying. Go helper
nonzero exits provide exit code plus manual inspection guidance, not captured
stderr. Install does not start immediately; choose Start with separate consent.
Uninstall is offered only after SCM reports Stopped and preserves settings.

`Bağlantıyı kes` persists disabled tunnel intent but keeps service running.
`Windows servisini durdur` stops SCM service, not persisted tunnel intent.
Automatic service boot/crash restart reapplies saved intent. Saving settings
alone preserves intent; if enabled, backend replaces running tunnel. GUI closing
does not stop service. Existing console agent/Startup VBScript is untouched;
user must migrate manually to avoid duplicate-ID conflict.

Secure defaults: unchecked insecure, required trusted 64-hex SHA-256 leaf pin.
Insecure requires explicit warning acceptance and stays visibly warned. Initial
token requires 16–4096 UTF-8 bytes; blank token on later saves retains stored
token. Token is never read back; GUI clears it after successful save. Settings
use backend DPAPI/state-file ACL, not GUI storage. GUI opens pipe with `0x12019b`
(no GENERIC_WRITE), verifies server PID equals running SCM PID before writing
any request, uses overlapped I/O with 4.5-second transaction bound, four bounded
pipe-busy attempts, and 16-KiB request/response limits. Native worker keeps
Flutter UI responsive; replies return on window thread. No TCP control port.

Validation ceiling: widget tests cover Android restore/no-autostart, Windows
redacted restore, secure defaults, blank-token retention, settings intent,
tunnel commands, service state gating, install consent/no-autostart, insecure
consent and denied IPC. Actual UAC/SCM mutation, cross-user ACL denial, malicious
pipe PID rejection and boot/crash recovery need separately approved disposable
VM installation. Never test these against current agent without approval.

2026-10-05 local validation: `flutter analyze` clean, `flutter test` 13 tests
passed, `flutter build windows --release` passed. Release GUI launched against
missing service, rendered dark UI and disabled settings correctly, then closed
normally. No service installation/removal/start/stop, current-agent termination,
VBScript change, reboot or deployment performed. Flutter tool automatically
adds a Windows analyzer exclusion; that out-of-scope edit was restored afterward.

## Android

Flutter ekranı, Kotlin `specialUse` foreground service ve mevcut Go agent.
VPN profili/root gerekmez; yalnız sunucudan gelen proxy trafiği taşınır.

## Derleme

Flutter stable, Android SDK/NDK, Java 17+ ve mobil modül için Go 1.26 gerekir.
Sunucu kök modülü Go 1.25 olarak kalır. Telefon hedefi Android arm64;
uygulamanın minimum sürümü Android 8.0 / API 26'dır. AAR komutundaki
`-androidapi 23`, uygulamanın `minSdk = 26` gereksinimini düşürmez.

```powershell
cd mobile
go install golang.org/x/mobile/cmd/gobind
$env:PATH = "$(go env GOPATH)\bin;$env:PATH"
go tool gomobile bind -target=android/arm64 -androidapi 23 -o homeproxy.aar .
cd ../app
flutter analyze
flutter test
flutter build apk --debug --target-platform android-arm64
adb install -r build/app/outputs/flutter-apk/app-debug.apk
```

`gomobile`, `mobile/go.mod` içindeki sabitlenmiş tool bağımlılığı üzerinden
çalıştırılır; `gobind` PATH üzerinde bulunmalıdır. Android SDK/NDK kurulu olmalı;
`ANDROID_HOME` SDK konumunu göstermelidir. AAR Git'e eklenmez.
Mevcut Gradle yapılandırmasında debug ve release APK'lar geliştirme/debug
anahtarıyla imzalanır; Actions release artifact'ı da mağaza dağıtımına hazır değildir.

## Bağlantı

Sunucu adresi, benzersiz agent ID, en az 16 bayt token ve sunucunun leaf
sertifikasının SHA-256 parmak izi girilir. Parmak izi güvenilir bağımsız yoldan
alınmalı; telefon bağlandığı sunucuyu otomatik güvenilir kabul etmez.
Varsayılan sertifika pin kontrolüdür. `TLS doğrulamasını atla (insecure)`
seçeneği kullanıcı risk onayından sonra pin gereksinimini kaldırır. TLS şifrelemesi
kalır ancak sunucu kimliği doğrulanmaz; sahte sunucu token'ı ele geçirebilir.
Seçenek diğer ayarlarla birlikte kaydedilir; açık olduğunda ekranda uyarı kalır.
Servis çalışırken (bağlanıyor veya bağlıyken) uygulamadan ayar kaydetme ve yeniden
başlatma reddedilir; önce bağlantıyı durdurun. Yeni ayarlar sonraki başlatmada
uygulanır. Başlat isteğinin kabulü, QUIC kimlik doğrulamasının tamamlandığı anlamına gelmez.

Sunucu otomatik sertifikasını `server-tls.pem` olarak state dizininde saklar.
Docker/Dokploy için `/var/lib/homeproxy` kalıcı volume'u korunmalıdır. Güvenilir
sunucu logundaki `server TLS certificate SHA-256:` değerini uygulamaya girin.
Volume korunursa restart'ta parmak izi değişmez. İlk eski sürümden geçişte yeni
kalıcı kimlik oluşur. Production mount/deploy otomatik yapılmaz.

Başlat kullanıcı uygulamadayken servisi açar. Flutter ekranı kapansa da servis
tüneli sahiplenir. Durdur veya bildirimdeki Durdur bağlantıyı kapatır.
Android bildirim izni istenir. Sunucu, agent ID, token, parmak izi ve insecure
seçimi Android Keystore AES-GCM ile şifreli tek kayıt olarak saklanır.
Başlat ayarları otomatik kaydeder; açılışta form geri yüklenir ama
bağlantı otomatik başlamaz. Normal APK güncellemeleri kayıtları korur; kaldırma
ve uygulama verilerini temizleme siler. Backup kapalıdır. Okuma hatasında kayıt
silinmez; yeniden giriş/kaydetme kullanıcı kararıdır.
Bildirim sabit ikon kullanır ve yalnız bağlantı durumu değişince güncellenir.
Otomatik boot/process restart yoktur.

Kayıtlı ayarlarla başlatma/durdurma için HomeProxy hızlı ayarlar düğmesi
eklenebilir. Uygulama, bildirim ve düğme aynı servisi yönetir; düğme yalnız
kimlik doğrulanmış tünelde aktif görünür. Kullanım ve cihaz doğrulama sınırları:
[ANDROID_QUICK_SETTINGS.md](../docs/ANDROID_QUICK_SETTINGS.md).

HyperOS pil ve otomatik başlatma ayarları fiziksel test gerektirir. Foreground
service kesintisiz çalışmayı garanti etmez. Mobil veri için Wi-Fi'yi kullanıcı
kapatır; uygulama ağı zorlamaz. Aynı Wi-Fi dış IP kotasını paylaşabilir.
`specialUse` mağaza yayını için ayrı policy incelemesi gerektirir.

Sunucu panelinde sabit SOCKS5 port–agent eşlemesi uygulanmıştır; eşlenmiş portun
TCP/UDP trafiği yalnız atanmış agent'a gider, agent yoksa başka cihaza geçmez.
Kurulum: [ana README](../README.tr.md#yönetim-paneli-ve-yönlendirme).
Android agent mevcut QUIC/TCP/UDP ve public hedef IP korumasını kullanır.
Tünel datagram sınırı 8 bayt tünel başlığı dahil 1452 bayttır; SOCKS5 başlığı da
bu bütçeden yer tüketir. Büyük paketler düşürülür; yol MTU'su ayrıca sınır koyabilir.
