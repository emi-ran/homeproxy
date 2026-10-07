# HomeProxy

**SOCKS5 trafiğini kimlik doğrulamalı QUIC tüneli üzerinden kendi cihazlarınızın internet bağlantısına yönlendirin.**

[English](README.md) · [Türkçe](README.tr.md)

HomeProxy, proxy giriş noktası ile internete çıkış noktasını ayırır. Sunucuyu bir VPS veya konteyner sunucusunda çalıştırın; kullanmak istediğiniz ağdan bir Windows ya da Android agent bağlayın. İstemciler sunucunun SOCKS5 portuna bağlanır; hedef bağlantıları ve DNS çözümlemesi VPS'te değil, agent üzerinde yapılır.

> **SOCKS5 portlarını özel ağda tutun.** SOCKS5 dinleyicisinde istemci kimlik doğrulaması yoktur. Paylaşılan token, SOCKS5 istemcilerini değil agent'ları doğrular. Proxy portlarını internete açmayın.

## İçindekiler

- [Çalışma mantığı](#çalışma-mantığı)
- [Derleme ve indirme](#derleme-ve-indirme)
- [Hızlı başlangıç](#hızlı-başlangıç)
- [Docker ve Dokploy](#docker-ve-dokploy)
- [Yönetim paneli ve yönlendirme](#yönetim-paneli-ve-yönlendirme)
- [Windows ve Android agent](#windows-ve-android-agent)
- [Yapılandırma referansı](#yapılandırma-referansı)
- [Güvenlik ve sınırlar](#güvenlik-ve-sınırlar)
- [Geliştirme ve doğrulama](#geliştirme-ve-doğrulama)

## Çalışma mantığı

```text
SOCKS5 istemcisi       HomeProxy sunucusu            HomeProxy agent         Hedef
(özel ağ)         --> (VPS / konteyner sunucusu) <=> (Windows / Android) --> (internet)
                      TCP proxy dinleyicisi         dışarıya QUIC bağlantısı
                      UDP relay                     hedef DNS + internet çıkışı
```

- **TCP:** SOCKS5 CONNECT istekleri QUIC stream'leri üzerinden aktarılır.
- **UDP:** SOCKS5 UDP ASSOCIATE trafiği QUIC datagram'ları üzerinden aktarılır. ENet tabanlı oyunlar gibi kullanım senaryolarını destekler; uyumluluk istemcinin SOCKS5 UDP desteğine, davranışına ve paket boyutuna bağlıdır.
- **Dışarıya bağlantı:** tüneli agent başlatır; agent'ın modeminde genellikle gelen bağlantılar için port açmak gerekmez.
- **Birden fazla cihaz:** bir SOCKS5 portunu belirli agent'a eşleyebilir veya varsayılan agent seçimini kullanabilirsiniz.
- **TLS kimliği:** sunucu kendi sertifikasını oluşturup kalıcı saklayabilir. Windows GUI ve Android SHA-256 sertifika pin doğrulamasını, CLI ise CA tabanlı doğrulamayı destekler.
- **Hedef koruması:** agent üzerinde public olmayan hedef adresler varsayılan olarak engellenir.

Bu bir uygulama proxy'sidir, sistem genelinde çalışan VPN değildir. Yalnız SOCKS5'e gönderilen trafik tünelden geçer.

## Derleme ve indirme

CLI/sunucu modülü **Go 1.25 veya üzerini**, ayrı mobil modül **Go 1.26** gerektirir. Flutter ve platform SDK'ları yalnız GUI/APK derlemesi için gereklidir.

```sh
go mod download
go build -trimpath -o bin/homeproxy ./cmd/homeproxy
```

Windows'ta CLI/GUI başlatıcısını PowerShell ile derleyin:

```powershell
go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy
```

POSIX shell üzerinden çapraz derleme:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-linux-amd64 ./cmd/homeproxy
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-windows-amd64.exe ./cmd/homeproxy
```

Manuel [GitHub Actions iş akışı](.github/workflows/build.yml), Windows GUI, Android arm64 APK ve CLI derlemeleri sunar. Deponun Actions sekmesinden **HomeProxy Build Workflows** çalıştırıp ilgili çalışmanın artifact'larını indirin. Windows GUI paketinin tamamını çıkarın; DLL dosyaları ve `data/` dizini gereklidir.

Yerel GUI/APK derleme ve paketleme adımları: [app/README.md](app/README.md).

## Hızlı başlangıç

Bu örnek Linux sunucu ve Windows CLI agent kullanır. `proxy.example.com` yerine sunucunuzun erişilebilir adresini yazın. Sunucuda gelen **UDP 4433** trafiğine izin verin; TCP 1080'i loopback veya güvenilir özel ağda tutun.

### 1. Sunucuyu başlatın

İki cihazda da aynı, en az **16 bayt** uzunluğunda güçlü rastgele token kullanın. Shell veya secret manager üzerinden yükleyin; Git'e eklemeyin.

```sh
export HOMEPROXY_TOKEN='REPLACE_WITH_A_STRONG_RANDOM_TOKEN'
./bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080 -state-dir data -admin-socket "$PWD/data/admin.sock"
```

İlk başlangıçta sunucu, sertifikayı ve private key'i içeren `data/server-tls.pem` dosyasını oluşturur. Restart ve güncellemelerde bu dizini koruyun. Sunucu, leaf sertifikasının SHA-256 parmak izini loga yazar. Örnekte yönetim soketi bu yazılabilir dizinde tutulur; varsayılan `/run/homeproxy/` kullanılacaksa dizin önceden var olmalı ve yazılabilir olmalıdır.

### 2. Agent'ı sertifika doğrulamasıyla bağlayın

Otomatik self-signed sertifika için agent'a güvenilir yoldan **yalnız public sertifikayı**, `server-cert.pem` adıyla aktarın. Birleşik `server-tls.pem` dosyasını agent'a kopyalamayın: private key de içerir. Sunucuda public sertifikayı OpenSSL ile ayırabilirsiniz:

```sh
openssl x509 -in data/server-tls.pem -out server-cert.pem
```

Ardından Windows PowerShell'de:

```powershell
$env:HOMEPROXY_TOKEN = 'REPLACE_WITH_A_STRONG_RANDOM_TOKEN'
.\bin\homeproxy.exe agent -quic proxy.example.com:4433 -id home-pc -ca .\server-cert.pem -server-name homeproxy
```

`homeproxy`, otomatik sertifikadaki DNS adlarından biridir; `-server-name` ile açıkça belirtildiğinde tünel adresiyle aynı olması gerekmez. Kendi CA imzalı sertifikanız için gerçek sertifika DNS adını ve uygun güven köklerini kullanın.

Windows GUI ve Android'de CA sertifikası kurmak yerine güvenilir sunucu logundaki parmak izini girebilirsiniz. CLI `agent` komutunda parmak izi parametresi yoktur.

### 3. SOCKS5 üzerinden test edin

Bu komutu **sunucuda** çalıştırın veya aynı güvenilir özel ağdaki istemciye göre adresi değiştirin:

```sh
curl --socks5-hostname 127.0.0.1:1080 https://ifconfig.me
```

Dönen adres, agent ağının public çıkış IP'si olmalıdır. `--socks5-hostname`, hedef adını istemcide çözmek yerine proxy'ye gönderir. Bu test TCP çıkışını doğrular; UDP veya oyun uyumluluğunu doğrulamaz.

## Docker ve Dokploy

### Docker Compose

Depodaki [compose.yaml](compose.yaml), token dosyasını `/secrets/token` konumuna bağlar, TLS durumunu kalıcı saklar ve yalnız UDP 4433'ü yayınlar. Yönetim panelini **etkinleştirmez**.

1. Paylaşılan token'ı içeren `secrets/token` dosyasını oluşturun. Konteynerde UID 10001'in okuyabildiğinden emin olun; diğer kullanıcıların erişimini kısıtlayın.
2. İsteğe bağlı olarak kökteki `.env` dosyasına `HOMEPROXY_MAX_AGENTS=5` ekleyin. Değer yoksa veya boşsa Compose varsayılanı 2'dir.
3. Servisi başlatın:

```sh
docker compose up -d --build
docker compose logs -f homeproxy
```

İstemci konteynerlerini uygun ortak özel ağa bağlayıp `homeproxy:1080` kullanın. TCP 1080'i yayınlamayın. UDP kullanan istemciler dinamik atanmış UDP relay portlarına da erişebilmelidir; yalnız TCP 1080 erişimi yeterli değildir.

### Dokploy Application

**Dockerfile tabanlı Application** kullanın. Dokploy Application ayarları, yerel Compose yapılandırmasından bağımsızdır.

| Ayar | Değer / gereksinim |
| --- | --- |
| Kaynak | Bu depo ve seçtiğiniz branch |
| Build type | Kökteki `Dockerfile` |
| Environment | `HOMEPROXY_TOKEN`, `HOMEPROXY_PANEL_PASSWORD`; isteğe bağlı `HOMEPROXY_MAX_AGENTS` |
| Dışarı açık transport | `4433:4433/udp`, host mode |
| SOCKS5 | 1080 ve ek eşlenmiş portları özel konteyner ağında tutun |
| Panel domain | Konteyner portu 3000'e HTTPS reverse proxy; public düz HTTP olarak açmayın |
| Kalıcı mount | `/var/lib/homeproxy` konumunda `homeproxy-state` named volume |

Token ve **ayrı panel parolası** en az 16 bayt olmalıdır. Dockerfile varsayılan olarak paneli etkinleştirir; eksik/kısa panel parolası başarılı başlangıcı engeller.

Redeploy sırasında volume'u koruyun. Bind mount kullanıyorsanız UID 10001 yazabilmelidir. Bağımsız sunucular arasında aynı state volume'u paylaşmayın. İstemciler, deploy'da görünen gerçek servis adını kullanmalıdır; örneğin `homeproxy-service:1080`. SOCKS kullanıcı adı ve parola alanlarını boş bırakın.

## Yönetim paneli ve yönlendirme

Panel bağlı agent'ları gösterir ve sabit SOCKS5 port–agent eşlemeleri eklemenizi/güncellemenizi sağlar. Yerel sunucuda açıkça etkinleştirin:

```sh
export HOMEPROXY_PANEL_PASSWORD='REPLACE_WITH_A_SEPARATE_RANDOM_PASSWORD'
./bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080 -state-dir data -admin-socket "$PWD/data/admin.sock" -panel 127.0.0.1:3000
```

Yukarıda ayarlanan paylaşılan token da gereklidir. Uzaktan erişim için HTTPS reverse proxy kullanın; düz HTTP dinleyicisini doğrudan yayınlamayın.

Örnek eşlemeler:

| SOCKS5 portu | Agent ID |
| --- | --- |
| 1080 | `home-pc` |
| 1081 | `phone` |

- ID, agent yapılandırmasıyla birebir eşleşmelidir.
- Eşlenmiş port, TCP ve UDP için yalnız o agent'ı kullanır. Agent çevrimdışıysa başka cihaza geçmek yerine istek başarısız olur.
- Değişiklikler mevcut bağlantıları değil, yeni oturumları etkiler.
- State dizinindeki `routes.json` dosyasında en fazla 32 eşleme tutulur. Panel şu anda ekleme/güncelleme sunar; silme sunmaz.
- Eşlenmemiş varsayılan port, sunucunun agent seçim davranışını kullanır. Çıkış cihazının sabit kalması gerekiyorsa 1080'i de açıkça eşleyin.
- Gösterilen agent IP'leri sunucunun gördüğü QUIC bağlantı adresleridir; hedef tarafta bağımsız doğrulanmış çıkış IP'leri değildir.

Panel oturumları sekiz saat sonra veya sunucu restart'ında biter. Yönetim işlemleri CSRF token gerektirir; HTTPS oturumlarında Secure, HttpOnly, SameSite=Strict cookie kullanılır. Giriş denemeleri genel hız sınırına tabidir. Bu önlemler public SOCKS5 dinleyicisini güvenli hale getirmez.

## Windows ve Android agent

### Windows GUI ve servis

Tam Windows GUI paketinden `homeproxy.exe` dosyasını parametresiz çalıştırın. Yanındaki `homeproxy-gui.exe` açılır; tek başına CLI binary'si GUI içermez.

GUI, ayrı tünel başlatmak yerine `HomeProxyAgent` Windows servisini yönetir. Servis kurma, kaldırma, başlatma ve durdurma işlemleri kullanıcı onayı ve UAC gerektirir. Servis düşük yetkili sanal hesapla çalışır, otomatik başlar; ayarlar DPAPI ve dosya ACL'leriyle korunur.

- Sunucu adresini, benzersiz agent ID'yi, token'ı ve güvenilir SHA-256 parmak izini girin.
- GUI'yi kapatmak servisi veya tüneli durdurmaz.
- Tünel bağlantısını kesmek, kapalı bağlantı tercihini kalıcı saklar; Windows servisini durdurmak ayrı işlemdir.
- Konsol agent veya eski Startup script'inden geçiyorsanız eski kurulumu kendiniz kaldırın/durdurun. Installer otomatik yapmaz.

Servis komutları, yetkiler, paketleme ve doğrulama sınırları: [Windows agent belgesi](docs/WINDOWS_AGENT.md). Tam SCM/UAC ve boot/crash recovery doğrulaması, gözden çıkarılabilir bir Windows test ortamı gerektirir.

### Android

Flutter uygulaması Kotlin foreground service ve Go tünel çekirdeğini kullanır. Root veya VPN profili gerektirmez; yalnız sunucunun istediği proxy trafiğini taşır.

1. Sunucu adresini, benzersiz agent ID'yi, token'ı ve güvenilir sertifika parmak izini girin.
2. Uygulamadan agent'ı başlatın. Ayarlar Android Keystore ile şifrelenir ve sonraki açılışlarda geri yüklenir; geri yükleme otomatik bağlantı başlatmaz.
3. İsteğe bağlı olarak HomeProxy düğmesini hızlı ayarlara ekleyip kayıtlı ayarlarla başlatma/durdurma yapın.

Bağlantı ayarlarını değiştirmeden önce tüneli durdurun. Uygulama, bildirim ve hızlı ayarlar düğmesi aynı servisi yönetir. Boot veya süreç ölümünden sonra otomatik bağlantı yoktur; Android/OEM pil politikaları arka plandaki çalışmayı kesebilir.

Derleme: [app/README.md](app/README.md). Düğme davranışı ve bekleyen cihaz doğrulamaları: [Hızlı ayarlar kılavuzu](docs/ANDROID_QUICK_SETTINGS.md).

## Yapılandırma referansı

CLI, `.env` dosyasını **otomatik yüklemez**. Compose değişken çözümlemesi ve Dokploy Environment ayarları ayrı mekanizmalardır.

### Ortam değişkenleri

| Değişken | Amaç / varsayılan |
| --- | --- |
| `HOMEPROXY_TOKEN` | Agent kimlik doğrulama token'ı; en az 16 bayt |
| `HOMEPROXY_MAX_AGENTS` | Eşzamanlı agent limiti; değişken tanımlı değilse 2; pozitif tamsayı olmalı |
| `HOMEPROXY_PANEL_PASSWORD` | Ayrı panel parolası; panel açıksa en az 16 bayt |
| `HOMEPROXY_CERT` / `HOMEPROXY_KEY` | İsteğe bağlı sertifika/key PEM **içeriği**; ikisini birlikte verin |

Token önceliği: `-token`, ardından `-token-file`, ardından `HOMEPROXY_TOKEN`. Komut satırındaki sırlar süreç listesinde veya shell geçmişinde görünebilir; günlük kullanımda ortam değişkeni/secret dosyası tercih edin. `-cert`/`-key` dosya yolları, sertifika ortam değişkenlerinden önceliklidir. Geçersiz sertifika girdisi, sessizce yeni sertifika üretmek yerine başlangıcı durdurur.

Agent limiti başlangıçta okunur; değişiklik restart gerektirir. Mevcut ID ile yeniden bağlantı, limit dolu olsa da o ID'nin eski bağlantısını değiştirir. Ayrı cihazlara ayrı ID verin.

### Temel CLI seçenekleri

| Seçenek | Kullanıldığı mod | Varsayılan / amaç |
| --- | --- | --- |
| `-quic` | server, agent | `127.0.0.1:4433`; dinleme veya uzak tünel adresi |
| `-token-file` | server, agent, select | Token'ı dosyadan oku |
| `-token` | server, agent, select | Açık token; günlük sır yönetiminde tercih etmeyin |
| `-socks` | server | `127.0.0.1:1080`; SOCKS5 bind adresi |
| `-state-dir` | server | `data`; kalıcı sertifika ve panel yönlendirme durumu |
| `-panel` | server | Varsayılan kapalı; özel HTTP panel bind adresi |
| `-cert` / `-key` | server | PEM sertifika/key dosya yolları; birlikte verin |
| `-mode` | server | `priority`; `priority`, `automatic`, `manual` kabul edilir |
| `-id` | agent, select | Gerekli agent ID / seçim hedefi |
| `-priority` | agent | `100`; küçük sayı önceliklidir |
| `-ca` | agent | CA PEM dosyası; verilmezse sistem güven kökleri |
| `-server-name` | agent | Doğrulamalı CLI TLS için gerekli sertifika DNS adı |
| `-insecure` | agent | `false`; sunucu kimliğini doğrulamaz, güvenilmeyen ağlarda tehlikelidir |
| `-allow-private` | agent | `false`; test fixture'ları için tehlikeli özel hedef izni |
| `-admin-socket` | server, select | `/run/homeproxy/admin.sock`; yerel Unix yönetim soketi |

Eşlenmemiş portlarda `priority`, en düşük öncelik değerini seçer; eşitlikte ID belirleyicidir. `automatic`, seçilmiş agent erişilebilirken onu korur. `select` ile yapılan seçim önceliklidir. Mevcut `manual` modu da seçilmiş agent yoksa priority seçimine döner; cihaz değişmesi kabul edilemiyorsa sabit panel eşlemeleri kullanın.

Unix soketi destekleyen bir sunucuda bağlı agent'ı yerel olarak seçin:

```sh
./bin/homeproxy select -id home-pc -admin-socket "$PWD/data/admin.sock"
```

Sokete erişim yanında paylaşılan token da gereklidir. CLI seçenek yardımı için `homeproxy server -h` veya `homeproxy agent -h` kullanın.

## Güvenlik ve sınırlar

- **İstemci ağı güvenilir olmalı.** İstemci–sunucu SOCKS5 trafiğini HomeProxy ayrıca şifrelemez veya doğrulamaz. QUIC, sunucu–agent tünelini korur.
- **Sunucu kimliğini doğrulayın.** Sertifika/parmak izini bağımsız güvenilir kanaldan alın. `-insecure`, şifrelemeyi korur fakat sahte sunucu token'ı ve trafiği ele geçirebilir; sertifika hatalarını çözmek için güvenli bir yöntem değildir.
- **Kalıcı durumu koruyun.** `server-tls.pem` private key içerir. Kaybı sunucu parmak izini değiştirir; agent güvenini açıkça güncelleyin. Bozuk durum sessizce yeniden oluşturulmaz.
- **Hedef korumasını açık tutun.** Private, loopback, link-local ve shared adres aralıkları varsayılan engellenir. Bu bir savunma katmanıdır; güvenilen istemcilerin tüm kötüye kullanımına karşı garanti değildir.
- **UDP boyutu sınırlıdır.** Mevcut tünel datagram limiti, 8 bayt tünel başlığı dahil 1452 bayttır. Büyük paketler tünel tarafından parçalanmaz, düşürülür. Yol MTU'su ve SOCKS5 ek yükü kullanılabilir payload boyutunu etkiler.
- **Hesap başına kota yoktur.** Agent limiti ve sabit yönlendirme, üçüncü taraf oyun/hesap limitlerini uygulamaz veya benzersiz public IP sağlamaz. Aynı ağdaki cihazlar çıkış IP'sini paylaşabilir.
- **Kesintisizlik garantisi yoktur.** NAT değişimleri, firewall, mobil pil yönetimi ve transport koşulları tüneli kesebilir. Fiziksel cihaz ve deploy ortamına özel test gerekir.

## Geliştirme ve doğrulama

Depo kökünden:

```sh
go test ./...
go vet ./...
go test -race ./...
python scripts/check_android_tile.py
python scripts/check_android_restart.py
```

Race detector desteklenen platform ve C toolchain gerektirir. Python kontrolleri Android kaynak sözleşmelerini inceler; Kotlin/APK derlemesi veya cihaz testi yerine geçmez. Mobil modülün ve Flutter uygulamasının ayrı kontrolleri [app/README.md](app/README.md) içinde açıklanır.

| Yol | İçerik |
| --- | --- |
| `cmd/homeproxy/` | CLI giriş noktası ve Windows başlatıcısı |
| `internal/proxy/` | QUIC tüneli, SOCKS5, yönlendirme, panel, hedef kontrolleri |
| `internal/windowsagent/` | Windows servisi, şifreli ayarlar, yerel IPC |
| `mobile/` | Go mobil binding'leri; ayrı Go modülü |
| `app/` | Flutter UI ve native Android/Windows entegrasyonu |
| `scripts/` | Kaynak sözleşmesi doğrulama script'leri |
| `docs/` | Platform kılavuzları, tasarım notları ve geçmiş doğrulama kayıtları |

Geçmiş doğrulamalar ve platform sınırları [VERIFY.md](docs/VERIFY.md), [PANEL_VERIFY.md](docs/PANEL_VERIFY.md), [ANDROID_VERIFY.md](docs/ANDROID_VERIFY.md) ve [WINDOWS_AGENT.md](docs/WINDOWS_AGENT.md) içinde kayıtlıdır. Bunlar belirli kontrollerin kayıtlarıdır; her deploy'un veya güncel derlemenin test edildiğinin kanıtı değildir.
