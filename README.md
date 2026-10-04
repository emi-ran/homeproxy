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
3. **Sıfır Yapılandırma (Zero-Config TLS)**: Sunucu, RAM üzerinde otomatik TLS 1.3 sertifikası üretir. Harici domain veya Let's Encrypt sertifikası gerekmez.

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

HomeProxy, Dokploy üzerinde **Application** (Docker Swarm) olarak çalışacak şekilde optimize edilmiştir.

### Dokploy Panelinde Ayarlar:
1. **Kaynak (Source)**: GitHub reponuzu seçin (`main` veya `master` branch).
2. **Build Type**: `Dockerfile` (Proje kökündeki çok-aşamalı minimal Dockerfile otomatik kullanılır).
3. **Environment (Ortam Değişkenleri)**:
   ```env
   HOMEPROXY_TOKEN="REPLACE_WITH_A_STRONG_RANDOM_TOKEN"
   ```
4. **Port Yapılandırması**:
   - `4433:4433/udp` ➡️ **Host Mode** seçin (Evdeki agent'ın QUIC tüneliyle bağlanabilmesi için dışarı açık olmalı).
   - `1080` ➡️ **Yalnızca Dahili Ağ** (Dışarı port mapping yapmayın! SOCKS5 dışarıdan şifresiz taranmamalı, sadece Dokploy iç ağındaki botlar erişmelidir).
5. **Deploy**: **Deploy** butonuna tıklayın.

Sunucu ayağa kalktığında `docker logs` üzerinde şunu görmelisiniz:
```text
zero-config: generated self-signed TLS 1.3 certificate
server ready
```

---

## 2. Kendi Bilgisayarınız (Windows Agent Kurulumu)

Evdeki Windows bilgisayarınız trafiğin internete çıkacağı uç noktadır.

### Manuel Çalıştırma
PowerShell açıp doğrudan çalıştırabilirsiniz:
```powershell
.\bin\homeproxy.exe agent -quic 203.0.113.10:4433 -id ev-pc -insecure -token "REPLACE_WITH_A_STRONG_RANDOM_TOKEN"
```

---

### PC Açıldığında Otomatik & Arka Planda Başlatma (Tavsiye Edilen)

Bilgisayarınızı her açtığınızda **siyah konsol ekranı açılmadan**, tamamen arka planda sessiz sedasız çalışması için Windows Başlangıç klasörüne gizli bir VBScript kaydedebilirsiniz:

#### Tek Komutla Kurulum:
PowerShell'i açın ve kendi VDS IP'nizi / Token'ınızı yazarak yapıştırın:

```powershell
$vbsPath = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\homeproxy-agent.vbs"
$cmd = '"""C:\Users\YOUR_USER\Documents\GitHub\homeproxy\bin\homeproxy.exe"" agent -quic 203.0.113.10:4433 -id ev-pc -insecure -token REPLACE_WITH_A_STRONG_RANDOM_TOKEN'
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
