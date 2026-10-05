# HomeProxy 🚀

> **Headless SOCKS5 over Authenticated QUIC Tunnel**  
> Güvenli, ultra hafif ve merkezi sunucu üzerinden doğrudan çıkış yapmayan; tüm HTTP, TCP ve UDP (ENet / Oyun) trafiğini yetkilendirilmiş ev/uç istemciler (agent) üzerinden tünelleyerek yönlendiren modern bir SOCKS5 proxy çözümü.

---

## 📑 İçindekiler
- [Mimari ve Çalışma Mantığı](#-mimari-ve-çalışma-mantığı)
- [Öne Çıkan Özellikler](#-öne-çıkan-özellikler)
- [Derleme Kılavuzu (Build)](#-derleme-kılavuzu-build)
- [1. Dokploy / VDS Sunucu Kurulumu](#1-dokploy--vds-sunucu-kurulumu)
- [2. Kendi Bilgisayarınız (Windows Agent Kurulumu)](#2-kendi-bilgisayarınız-windows-agent-kurulumu)
  - [Manuel Çalıştırma](#manuel-çalıştırma)
  - [PC Açıldığında Otomatik & Arka Planda Başlatma (Tavsiye Edilen)](#pc-açıldığında-otomatik--arka-planda-başlatma-tavsiye-edilen)
  - [Agent'ı Durdurma](#agentı-durdurma)
- [3. Dokploy Konteynerlerinde (Mori / Botlar) Kullanım](#3-dokploy-konteynerlerinde-mori--botlar-kullanım)
- [CLI Parametreleri Referansı](#-cli-parametreleri-referansı)
- [Test ve Doğrulama](#-test-ve-doğrulama)
- [Güvenlik Prensipleri](#-güvenlik-prensipleri)

---

## 📌 Mimari ve Çalışma Mantığı

HomeProxy, hedef adresleri sunucu (VDS/VPS) üzerinden **çözümlemez ve sunucudan çıkış yapmaz**.

```
 +------------------------+              +----------------------+              +--------------------+              +---------------+
 | Dokploy Bot Konteyneri |   SOCKS5     |   HomeProxy Server   |     QUIC     |  Ev Bilgisayarı    |    Doğrudan  |   Hedef Web   |
 |  (Mori / Growtopia)    | -----------> |   (VDS / Dokploy)    | <=========== |   (Windows Agent)  | ------------> |    / Oyun     |
 | 10.0.1.175:1080 (TCP)  | (TCP & ENet) |   Port: 4433/udp     |  (TLS 1.3)   |  Residential IP    |               |  Sunucuları   |
 | Ephemeral Relay (UDP)  |              |  (Zero-Config TLS)   |              | (192.0.2.10)  |               | (198.51.100.20) |
 +------------------------+              +----------------------+              +--------------------+              +---------------+
```

1. **Sunucu (`server`)**: VDS üzerinde çalışır. SOCKS5 istemcilerini (TCP ve UDP) kabul eder ve evdeki agent ile QUIC (TLS 1.3) üzerinden şifreli bir tünel kurar.
2. **Uç Ajan (`agent`)**: Evdeki PC'de çalışır. Sunucuya dışarıdan içeri (outbound) bağlanır (modemde port açmaya gerek yoktur). Gelen istekleri ev internetinizin IP adresiyle hedefe bağlar.
3. **Sıfır Yapılandırma (Zero-Config TLS)**: Sunucu otomatik TLS 1.3 sertifikası üretip kalıcı `server-tls.pem` dosyasına saklar. Harici domain veya Let's Encrypt gerekmez. SHA-256 parmak izi başlangıç logunda gösterilir.

---

## ✨ Öne Çıkan Özellikler

- **QUIC & TLS 1.3 Taşıma Katmanı**: Düşük gecikmeli, paket kaybına dirençli UDP tüneli (`quic-go`).
- **ENet & Oyun UDP Desteği**: SOCKS5 UDP ASSOCIATE trafiği QUIC Datagram (RFC 9221) üzerinden taşınır. Growtopia vb. ENet tabanlı oyun protokolleriyle tam uyumludur.
- **Docker Swarm Overlay Desteği**: Dokploy iç ağında sanal IP (VIP) kısıtlamalarına takılmadan konteynerler arası dinamik UDP yönlendirmesini otomatik çözer (`getRelayIP`).
- **Anti-SSRF Güvenlik Kalkanı**: Ev bilgisayarınızın yerel ağına (`192.168.x.x`, `10.x.x.x`, `127.0.0.1`) veya bulut metadata servislerine proxy üzerinden izinsiz erişim engellenir.
- **Doğrudan Token Girişi (`-token`)**: Ortam değişkeniyle uğraşmadan parametre olarak parola geçebilme kolaylığı.
- **Sessiz Tekil Örnek Koruması (Single-Instance Mutex)**: Aynı ID ile çalışan bir agent zaten varsa, ikinci kez açıldığında hiçbir hata veya pencere açmadan kendini anında sessizce kapatır (`ExitCode 0`). Çakışma ve çoklu kopya oluşmasını %100 engeller.

---

## 🛠️ Derleme Kılavuzu (Build)

Android Flutter agent geliştirmesi: [app/README.md](app/README.md).
Telefon ve sabit proxy portları fikri: [docs/PHONE_AGENT_PLAN.md](docs/PHONE_AGENT_PLAN.md).

Gereksinim: **Go 1.25+**

```bash
# Proje kök dizininde bağımlılıkları yükleyin
go mod download
```

### Windows İçin Derleme (`.exe`)
```powershell
# Windows üzerinde çalışıyorsanız doğrudan:
go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy

# Linux/macOS üzerinden Windows için çapraz derleme (Cross-Compile):
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy
```

### Linux (VDS / Sunucu) İçin Derleme
```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy ./cmd/homeproxy
```

---

## 1. Dokploy / VDS Sunucu Kurulumu

### Web panel ve port–agent eşlemesi

Giriş ekranı ve sekme başlığı nötrdür; HomeProxy adı yalnız doğrulanmış oturumda
gösterilir. Bu görsel tercih, servis gizleme veya güvenlik garantisi değildir.
Panel mobil uyumlu koyu tema ve çevrimiçi agent ID önerileri kullanır.
Giriş sonrası cihazların QUIC bağlantı IP'leri gösterilir; aynı IP kullanan
cihazlar işaretlenir. Bu, sunucunun gördüğü bağlantı IP'sidir, oyun hedefindeki
çıkış IP'sinin kesin ölçümü değildir. Ağ değişiminden sonra sayfayı yenileyin.

Dockerfile sunucuyla birlikte paneli `3000/tcp` üzerinde başlatır. Dokploy
Application **Environment** alanına ayrı, en az 16 bayt
`HOMEPROXY_PANEL_PASSWORD` ekleyin. Şifre yoksa Docker varsayılan başlangıcı hata
verir; kullanıcı adı gerekmez. Şifreyi image'a veya Git'e eklemeyin. Yerel CLI
paneli `-panel 127.0.0.1:3000` ile açar; ortam değişkenini kendiniz yükleyin,
uygulama `.env` dosyasını otomatik okumaz.

Dokploy Domain ayarında container port `3000` ve HTTPS kullanın. Panel portunu
hosta doğrudan HTTP olarak açmayın. Cookie HTTPS erişiminde Secure/HttpOnly,
SameSite Strict; yönetim işlemleri CSRF token gerektirir. Oturumlar 8 saat sonra
ve sunucu restart'ında biter. Giriş denemeleri toplam saniyede bir ile sınırlı.

Kalıcı named volume `homeproxy-state` için mount path `/var/lib/homeproxy`
ekleyin. Panelden `1080 = ev-pc`, `1081 = telefon` kaydedilebilir. Agent ID'leri
cihazlarda kullanılan değerlerle birebir eşleşmeli. TCP CONNECT ve UDP ASSOCIATE
seçilen agent'a sabitlenir; agent yoksa başka cihaza failover yapılmaz. Eşleme
değişikliği yalnız yeni oturumlara uygulanır. Portlar SOCKS bind adresinde açılır,
yalnız iç ağda kalmalı. En fazla 32 eşleme; panel bu sürümde ekleme/güncelleme
sunar, silme sunmaz. `routes.json` volume'da saklanır ve başlangıçta yüklenir.
Eşlenmemiş varsayılan `1080` portu eski priority davranışını korur; kullanmadan
önce onu da PC'ye eşleyin. Üç hesap kotası otomatik uygulanmaz.

Compose Dokploy Application deploy'unda kullanılmaz. Bu ayarlar Dokploy
Environment, Domain ve Mounts alanlarından yapılmalıdır.

HomeProxy, Dokploy üzerinde **Application** (Docker Swarm) olarak çalışacak şekilde optimize edilmiştir.

### Dokploy Panelinde Ayarlar:
1. **Kaynak (Source)**: GitHub reponuzu seçin (`main` veya `master` branch).
2. **Build Type**: `Dockerfile` (Proje kökündeki çok-aşamalı minimal Dockerfile otomatik kullanılır).
3. **Environment (Ortam Değişkenleri)**:
   ```env
    HOMEPROXY_TOKEN="REPLACE_WITH_A_STRONG_RANDOM_TOKEN"
    HOMEPROXY_MAX_AGENTS=5
   ```
4. **Port Yapılandırması**:
   - `4433:4433/udp` ➡️ **Host Mode** seçin (Evdeki agent'ın QUIC tüneliyle bağlanabilmesi için dışarı açık olmalı).
   - `1080` ➡️ **Yalnızca Dahili Ağ** (Dışarı port mapping yapmayın! SOCKS5 dışarıdan şifresiz taranmamalı, sadece Dokploy iç ağındaki botlar erişmelidir).
5. **Deploy**: **Deploy** butonuna tıklayın.

Sunucu ayağa kalktığında `docker logs` üzerinde şunu görmelisiniz:
```text
zero-config: loaded persistent self-signed TLS 1.3 certificate
server TLS certificate SHA-256: <64 hex karakter>
server ready
```

Eşzamanlı agent limiti varsayılan **2**; `HOMEPROXY_MAX_AGENTS` pozitif tamsayı
olmalıdır. Boş, sıfır, negatif, sayısal olmayan veya platformun `int` aralığını
aşan değer sunucu başlangıcını hata ile durdurur. Aynı ID yeniden bağlanınca eski
bağlantı değiştirilir; limit doluyken de reconnect çalışır. En fazla dört bekleyen
kimlik doğrulama ayrı sınırlanır; aktif agent'lar bu slotları tutmaz. Bekleyen
slotların kötüye kullanımı reconnect'i geçici engelleyebilir; DoS garantisi yoktur.

Dokploy Application için **Environment** alanına `HOMEPROXY_MAX_AGENTS=5` ekleyin;
yerel `.env` production ayarını değiştirmez. Yerel Docker Compose, kökteki `.env`
dosyasından `HOMEPROXY_MAX_AGENTS=5` değerini konteynere geçirir; ayar yoksa/boşsa
Compose varsayılanı 2'dir. CLI `.env` otomatik yüklemez; doğrudan çalıştırırken
ortam değişkenini dışarıdan ayarlayın (PowerShell: `$env:HOMEPROXY_MAX_AGENTS='5'`).
Değişiklik çalışan sunucuya canlı uygulanmaz; sonraki başlangıçta okunur.

---

## 2. Kendi Bilgisayarınız (Windows Agent Kurulumu)

### Android için sertifika parmak izi

Dokploy Application'a **named volume** ekleyin: `homeproxy-state`, mount path
`/var/lib/homeproxy`. Güncellemelerde volume'u koruyun; birden fazla bağımsız
HomeProxy sunucusu aynı volume'u paylaşmamalı. Docker image bu dizini UID 10001
için hazırlar. Mevcut bind mount kullanılıyorsa dizin bu kullanıcıya yazılabilir
olmalıdır. Image güncellemesi ve mount ayarı deploy gerektirir.

Sunucu logundaki `server TLS certificate SHA-256:` sonrasındaki 64 karakteri
Android uygulamasının sertifika alanına girin. Parmak izini yalnız güvenilir
Dokploy panelinden alın. Token veya private key paylaşmayın.

Sertifika ve private key tek `server-tls.pem` dosyasında saklanır. Restart ve
redeploy sırasında volume korunursa parmak izi değişmez. Volume/dosya silinirse
kimlik değişir; yeni parmak izi telefona güvenilir yoldan girilmelidir.
Bozuk/okunamayan sertifika otomatik değiştirilmez; sunucu hata verir.
İlk geçişte eski RAM sertifikası korunamaz; yeni kalıcı sertifika oluşturulur.

CLI varsayılanı `-state-dir data`; Docker varsayılanı `/var/lib/homeproxy`.
Özel `-cert`/`-key` veya `HOMEPROXY_CERT`/`HOMEPROXY_KEY` kullanılıyorsa onlar
önceliklidir; yine parmak izi loglanır. İki değer birlikte verilmeli, geçersiz
sertifikada otomatik sertifikaya sessiz geçiş yapılmaz.

Evdeki Windows bilgisayarınız trafiğin internete çıkacağı uç noktadır.

### Manuel Çalıştırma
PowerShell açıp doğrudan çalıştırabilirsiniz:
```powershell
.\bin\homeproxy.exe agent -quic 203.0.113.10:4433 -id ev-pc -insecure -token "REPLACE_WITH_A_STRONG_RANDOM_TOKEN"
```

---

### PC Açıldığında Otomatik & Arka Planda Başlatma (Tavsiye Edilen)

Windows SCM backend hazır; Flutter GUI ayrı aşamada eklenir. Kurulum, yetki modeli,
DPAPI ayarları ve GUI IPC sözleşmesi: [docs/WINDOWS_AGENT.md](docs/WINDOWS_AGENT.md).
Servis `NT SERVICE\HomeProxyAgent` düşük yetkili sanal hesabıyla çalışır;
otomatik başlar, yalnız process crash sonrasında yeniden başlatılır. Kurulum ve
kaldırma UAC gerektirir. GUI normal kullanıcı olarak çalışır; token geri okunmaz.

Windows'ta parametresiz `homeproxy.exe`, yanındaki `homeproxy-gui.exe` dosyasını
açar; GUI paketlenmediyse açık hata verir. Kalıcı servis adı `homeproxy-service.exe`.
`agent/server/select` CLI korunur. GUI varsayılanı SHA-256 pin doğrulamasıdır;
insecure seçeneği yalnız açık kullanıcı tercihi ve güvenlik uyarısıyla kullanılmalıdır.

Eski Startup VBScript kurulumu otomatik silinmez, çalışan agent öldürülmez.
Geçişte eski başlangıç kaydını ve agent'ı kullanıcı kaldırmalıdır. Global mutex
aynı ID ile console/servis oturumlarının birlikte çalışmasını engeller.

#### Eski VBScript yöntemi (yeni kurulum için önerilmez)

#### Tek Komutla Kurulum:
PowerShell'i açın ve kendi VDS IP'nizi / Token'ınızı yazarak yapıştırın:

```powershell
$vbsPath = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\homeproxy-agent.vbs"
$cmd = '"""C:\HomeProxy\homeproxy.exe"" agent -quic example.com:4433 -id ev-pc -server-name example.com -token-file C:\HomeProxy\token.txt'
$content = "Set WshShell = CreateObject(`"WScript.Shell`")`r`nWshShell.Run `"$cmd`", 0, False"
[System.IO.File]::WriteAllText($vbsPath, $content)
```

> **Hemen Başlatmak İçin:**
> ```powershell
> wscript.exe "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\homeproxy-agent.vbs"
> ```

#### Durumu Kontrol Etme:
Agent'ın arka planda çalıştığını doğrulamak için:
```powershell
Get-Process -Name homeproxy
```
*(Görev Yöneticisi ➡️ Ayrıntılar sekmesinde `homeproxy.exe` olarak görünür).*

> **💡 Akıllı Tekil Örnek Koruması (Mutex):**  
> Bilgisayarınızda `homeproxy.exe` zaten arka planda çalışıyorsa; ister başlangıç scripti ister siz manuel olarak tekrar başlatsanız dahi ikinci kopya **hiçbir uyarı vermeden kendini sessizce anında kapatır (`ExitCode 0`)**. Sistemde daima tek ve kararlı bir agent kalır.

---

### Agent'ı Durdurma
Arka plandaki agent'ı sonlandırmak istediğinizde:
SCM servisinde `homeproxy.exe service stop` kullanın (yönetici yetkisi gerekir).
Kalıcı tünel kapatma için GUI IPC `disconnect` kullanır. Aşağıdaki eski komut
yalnız manuel/Startup agent içindir; servis crash recovery tetiklememelidir.
```powershell
Stop-Process -Name homeproxy -Force
```

**Otomatik Başlangıçtan Kaldırmak İçin:**
```powershell
Remove-Item "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\homeproxy-agent.vbs"
```

---

## 3. Dokploy Konteynerlerinde (Mori / Botlar) Kullanım

Dokploy üzerindeki Mori veya diğer bot araçlarında proxy alanına girmeniz gereken bilgi:

```text
growtopia-homeproxy-ixjh7l:1080
```
*(veya Mori otomatik kaydettiğinde Dokploy ağ IP'si: `10.0.1.175:1080`)*

- **Kullanıcı adı ve Şifre**: Boş bırakın.
- **Protokol**: SOCKS5 (TCP + UDP).

### Test Sonuçları:
Mori **Proxy Tester** çalıştırıldığında 3 adım da yeşil yanacaktır:
- [x] **SOCKS5**: Bağlantı tokalaşması başarılı.
- [x] **server_data**: HTTP/TCP üzerinden Growtopia sunucu verileri ev IP'nizle çekildi.
- [x] **ENet UDP**: Oyun içi UDP paketleri QUIC datagram üzerinden ev PC'niz ile çift yönlü aktarıldı.

### VDS Üzerinden Test Etme:
VDS terminalinden proxy'nin ev IP'niz üzerinden çıktığını doğrulamak için:
```bash
curl --socks5 127.0.0.1:1080 https://ifconfig.me
# Çıktı: Ev internetinizin IP adresi (Örn: 192.0.2.10)
```

---

## 📖 CLI Parametreleri Referansı

HomeProxy 3 farklı modda çalıştırılabilir: `server`, `agent` ve `select`.

### `agent` Parametreleri
| Parametre | Varsayılan | Açıklama |
|---|---|---|
| `-quic` | `127.0.0.1:4433` | Uzak QUIC sunucu adresi (`IP:port` veya `domain:port`) |
| `-token` | `""` | Kimlik doğrulama parolası (en az 16 karakter) |
| `-token-file`| `""` | Parolanın okunacağı dosya yolu |
| `-id` | `""` | Agent tanımlayıcısı (Örn: `ev-pc`) |
| `-priority` | `100` | Öncelik derecesi (küçük sayı daha yüksek önceliktir) |
| `-insecure` | `false` | Self-signed sertifikalı sunucularda sertifika onayını atlar |
| `-allow-private` | `false` | **Tehlikeli**: Agent'ın yerel IP adreslerine bağlanmasına izin verir |

### `server` Parametreleri
| Parametre | Varsayılan | Açıklama |
|---|---|---|
| `-quic` | `127.0.0.1:4433` | Agent'ların bağlanacağı genel QUIC dinleme portu |
| `-socks` | `127.0.0.1:1080` | SOCKS5 dinleme adresi |
| `-token` | `""` | Kimlik doğrulama parolası |
| `-mode` | `priority` | Seçim modu: `priority`, `automatic`, `manual` |
| `-cert` / `-key` | `""` | Özel TLS sertifika dosyaları (Boşsa self-signed üretilir) |

---

## 🧪 Test ve Doğrulama

Tüm testler ve yarış durumu (race detector) kontrolleri:

```bash
# Tüm birim ve entegrasyon testlerini çalıştırın
go test -v ./...

# Yarış durumu kontrolü
go test -race ./...

# Statik analiz
go vet ./...
```

---

## 🔒 Güvenlik Prensipleri

1. **SOCKS Portunu Asla Dış Dünyaya Açmayın**: SOCKS5 protokolü kimlik doğrulamasız kurulur. Bu port sadece `127.0.0.1` veya Docker'ın izole dahili ağında (`dokploy-network`) kalmalıdır.
2. **Güçlü Token**: Paylaşılan token en az 16 karakterden oluşmalı ve gizli tutulmalıdır.
3. **Anti-SSRF**: Ev PC'nizin bağlı olduğu yerel ağdaki modem, NAS veya yazıcılar proxy üzerinden gelebilecek isteklerden korunur.
