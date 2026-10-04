# HomeProxy 🚀

> **Headless SOCKS5 over Authenticated QUIC Tunnel**
> Güvenli, hafif ve merkezi sunucu üzerinden doğrudan çıkış yapmayan, trafiği yetkilendirilmiş ev/uç istemciler (agent) üzerinden yönlendiren modern bir SOCKS5 proxy çözümü.

---

## 📌 Genel Bakış ve Mimari

**HomeProxy**, geleneksel proxy sunucularından farklı olarak hedef adresleri sunucu (VDS/VPS) üzerinden **çözümlemez ve doğrudan bağlamaz**. Bunun yerine:

1. **Sunucu (`server`)**: Bulut/VDS üzerinde çalışır; SOCKS5 istemcilerini (TCP ve UDP) kabul eder ve agent'lar ile QUIC (TLS 1.3) üzerinden şifreli, çoklamalı (multiplexed) tünel kurar.
2. **Uç Ajan (`agent`)**: Evdeki bilgisayar veya yerel ağda çalışır; sunucuya dışarıdan içeri doğru (outbound) güvenli bir QUIC bağlantısı açar. Gelen bağlantı isteklerinde hedef DNS adreslerini çözer ve gerçek çıkışı kendi IP'si üzerinden yapar.
3. **Yönetim İstemcisi (`select`)**: Yerel Unix domain socket üzerinden sunucuya bağlanarak aktif olarak trafiği aktaran agent'ı anlık olarak değiştirebilir.

```
 +------------------+           +----------------------+           +------------------+
 |  SOCKS5 Client   |  (TCP)    |   HomeProxy Server   |  (QUIC)   | HomeProxy Agent  |  (Direct)   +-------------+
 | (Browser/Curl)   | --------> | (VDS / Public Cloud) | <======== | (Home PC / Edge) | ----------> | Destination |
 | 127.0.0.1:1080   | (UDP Asso)|   0.0.0.0:4433/udp   | (TLS 1.3) | Residential IP   |             | (Web/API)   |
 +------------------+           +----------------------+           +------------------+             +-------------+
                                           ^
                                           | Unix Socket (admin.sock)
                                    +-------------+
                                    | Local Admin | (homeproxy select -id pc-b)
                                    +-------------+
```

---

## ✨ Temel Özellikler

- **QUIC & TLS 1.3 Taşıma Katmanı**:
  - `quic-go` tabanlı düşük gecikmeli, paket kaybına dirençli bağlantı.
  - SOCKS5 TCP oturumları için çift yönlü (bidirectional) QUIC stream'leri.
  - SOCKS5 UDP ASSOCIATE trafiği için QUIC Datagram desteği (RFC 9221).
  - Katı TLS 1.3 sertifika ve DNS/SNI doğrulaması (güvensiz mod bulunmaz).
- **Esnek Yönlendirme Modları (`-mode`)**:
  - `priority`: En düşük sayısal önceliğe (priority) sahip agent seçilir. Öncelik eşitliğinde sözlük sırasına göre ID seçilir.
  - `automatic`: İlk seçim önceliğe göre yapılır; ardından agent sağlıklı kaldığı sürece ona sabitlenir (sticky). Çökme durumunda yedek agent'a geçer ve ona sabitlenir.
  - `manual`: Manuel seçim yapılana kadar öncelik bazlı yedek çalışır; yerel soket üzerinden seçim yapıldığında o agent aktif kalır.
- **Yerel Yönetim Soketi (`listenManagement`)**:
  - Dosya izinleri `0600` olan Unix domain socket üzerinden güvenli, paylaşımlı token doğrulaması ile dinamik agent seçimi (`homeproxy select`).
- **Gelişmiş Güvenlik ve Anti-SSRF Koruması**:
  - Agent çıkışlarında loopback, özel ağ (RFC 1918), paylaşımlı adres blokları (`100.64.0.0/10` - bulut metadata servisleri dahil), link-local ve multicast IP adresleri hem sayısal hem de DNS çözümlemelerinde otomatik olarak engellenir (`-allow-private` yalnızca testler içindir).
  - SOCKS5 arabirimi şifresizdir; bu nedenle yalnızca yerel veya izole özel ağlarda çalıştırılmak üzere tasarlanmıştır.
- **Sıkı Kaynak Sınırları**:
  - Maksimum 2 kimlik doğrulanmış agent, 4 bağlantı kabul yuvası (admission slots).
  - 128 eşzamanlı SOCKS oturumu ve 128 QUIC akışı.
  - QUIC datagram tavanı: 8 baytlık oturum ID'si dahil **1100 bayt**.
  - UDP akışı için 60 saniye hareketsizlik (idle) zaman aşımı (başarılı giden pakette yenilenir) ve 1 saatlik bağımsız mutlak oturum ömrü (hard lifetime).

---

## 📂 Proje Klasör Düzeni

Proje, Go standart proje düzenine (Standard Go Project Layout) uygun olarak yeniden organize edilmiş ve derli toplu hale getirilmiştir:

```text
homeproxy/
├── cmd/
│   └── homeproxy/
│       └── main.go                 # Uygulama CLI giriş noktası (server, agent, select)
├── internal/
│   └── proxy/
│       ├── address.go              # SOCKS5 adres kodlayıcı/çözücü ve güvenli hedef denetleyicisi
│       ├── agent.go                # Outbound agent döngüsü, stream işleyicisi ve çıkış bağlantıları
│       ├── cli.go                  # CLI bayraklarının (flags) ayrıştırılması ve komut yürütücüsü
│       ├── manage.go               # Unix domain socket sunucusu ve yönetim istekleri
│       ├── proxy.go                # Çekirdek veri yapıları (server, agentPeer) ve yönlendirme algoritmaları
│       ├── server.go               # QUIC agent dinleyicisi, slot kontrolü ve SOCKS5 sunucusu
│       ├── transport.go            # QUIC stream köprüleme (bridge) ve TCP yarım-kapanış (half-close)
│       ├── udp.go                  # SOCKS5 UDP ASSOCIATE geçişi ve QUIC datagram paketleyicisi
│       ├── integration_test.go     # Uçtan uca entegrasyon testleri (TLS, TCP/UDP echo, failover)
│       ├── manage_test.go          # Yönetim soketi kimlik doğrulama testleri
│       ├── proxy_test.go           # Yönlendirme ve sticky seçim birim testleri
│       ├── reply_test.go           # SOCKS5 yanıt kodları ve bağlı adres testleri
│       ├── security_cleanup_test.go# Anti-SSRF, sıfırlama (reset) ve bellek sızıntı testleri
│       └── signal_test.go          # Zarif kapatma (SIGTERM) testi
├── docs/
│   └── VERIFY.md                   # Doğrulama test geçmişi ve güvenlik inceleme notları
├── Dockerfile                      # Üretim için minimal çok-aşamalı (multi-stage) Docker yapısı
├── compose.yaml                    # Ağ izolasyonlu Docker Compose yapılandırması
├── go.mod                          # Go modül bağımlılıkları
├── go.sum                          # Modül sağlama toplamları (checksums)
└── README.md                       # Kapsamlı proje dokümantasyonu
```

---

## 🛠️ Kurulum ve Derleme

### Gereksinimler
- **Go**: 1.25 veya üzeri
- Linux / macOS / Windows desteği (Agent Windows üzerinde de sorunsuz çalışır)

### Yerel Olarak Derleme

```bash
# Bağımlılıkları kontrol edin
go mod download

# Linux / macOS ikilisini derleyin
go build -trimpath -o bin/homeproxy ./cmd/homeproxy

# Windows için çapraz derleme (cross-compile)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-windows-amd64.exe ./cmd/homeproxy
```

---

## 🚀 Kullanım Kılavuzu

Uygulama tek bir ikili dosya üzerinden üç farklı modda çalıştırılır: `server`, `agent` ve `select`.

> **Güvenlik Notu**: Kimlik doğrulama parolası (`token`) en az **16 bayt** olmalıdır. Token'ı komut satırı argümanı olarak geçmeyin; `HOMEPROXY_TOKEN` ortam değişkeni veya `-token-file` bayrağı ile sağlayın.

### 1. Sunucu Modu (`server`)

Sunucuyu QUIC ve SOCKS5 portları ile başlatır:

```bash
# Hızlı / Sıfır Yapılandırma (Zero-Config) - Sertifika gerekmez!
export HOMEPROXY_TOKEN="REPLACE_WITH_A_STRONG_RANDOM_TOKEN"
./bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080

# Veya Özel Sertifika ile Başlatma:
./bin/homeproxy server \
  -quic 0.0.0.0:4433 \
  -socks 127.0.0.1:1080 \
  -cert /etc/ssl/homeproxy/cert.pem \
  -key /etc/ssl/homeproxy/key.pem \
  -token-file "$HOME/.homeproxy/token" \
  -admin-socket "$HOME/.homeproxy/admin.sock" \
  -mode priority
```

| Parametre | Varsayılan | Açıklama |
|---|---|---|
| `-quic` | `127.0.0.1:4433` | Agent'ların bağlanacağı genel QUIC adresi/portu |
| `-socks` | `127.0.0.1:1080` | SOCKS5 istemcilerinin bağlanacağı özel adres |
| `-cert` | `""` | TLS sunucu sertifikası (PEM) - Boşsa otomatik üretilir (Zero-config) |
| `-key` | `""` | TLS sunucu özel anahtarı (PEM) - Boşsa otomatik üretilir |
| `-mode` | `priority` | Yönlendirme modu: `priority`, `automatic`, `manual` |
| `-token-file` | `""` | Paylaşımlı gizli anahtar dosyası |
| `-admin-socket`| `/run/homeproxy/admin.sock` | Yerel Unix yönetim soketi yolu |

---

### 2. Ajan Modu (`agent`)

Ev bilgisayarında veya yerel ağdaki sunucuda çalıştırılır. Sunucuya outbound QUIC bağlantısı kurar:

```bash
# Sıfır Yapılandırma Modu (Self-signed sunucu sertifikası için -insecure):
./bin/homeproxy agent -quic VDS_IP:4433 -id home-pc -insecure

# Özel Domain ve Sertifika ile:
./bin/homeproxy agent \
  -quic proxy.example.com:4433 \
  -server-name proxy.example.com \
  -id home-pc-a \
  -priority 10 \
  -token-file token
```

| Parametre | Varsayılan | Açıklama |
|---|---|---|
| `-quic` | `127.0.0.1:4433` | Uzak QUIC sunucu adresi (`domain:port` veya `ip:port`) |
| `-server-name` | `""` | TLS sertifikasındaki DNS adı (SNI doğrulaması) |
| `-id` | `""` | Agent benzersiz tanımlayıcısı |
| `-priority` | `100` | Öncelik derecesi (düşük sayı daha yüksek önceliktir) |
| `-insecure` | `false` | **Zero-Config**: Sunucu sertifika doğrulamasını atlar (Self-signed modda gerekir) |
| `-ca` | `""` | Özel CA sertifikası yolu (boş bırakılırsa sistem kökleri kullanılır) |
| `-allow-private`| `false` | **DİKKAT**: Özel/yerel IP çıkışına izin verir (yalnızca test ortamları için) |

---

### 3. Yönetim Modu (`select`)

Sunucu üzerindeki aktif agent seçimini manuel olarak değiştirmek için kullanılır:

```bash
./bin/homeproxy select \
  -id home-pc-b \
  -token-file "$HOME/.homeproxy/token" \
  -admin-socket "$HOME/.homeproxy/admin.sock"
```

İşlem başarılı olduğunda ekrana `ok` yazdırılır; yetkisiz veya geçersiz isteklerde hata verilir.

---

## 🐳 Docker ve Compose Kullanımı

Depoda hazır bulunan `Dockerfile` ve `compose.yaml` ile güvenli ve yalıtılmış bir sunucu konteyneri çalıştırabilirsiniz:

```bash
docker compose up -d
```

### Konteyner Ağ Güvenliği
- Konteyner yalnızca **UDP 4433** portunu dış dünyaya açar (`ports: ["4433:4433/udp"]`).
- `socks` portu (1080) dış dünyaya açılmaz; yalnızca `private` dahili Docker ağına bağlı diğer servisler tarafından erişilebilir.
- Dosya sistemi salt okunurdur (`read_only: true`); yetkiler en aza indirilmiştir (`cap_drop: [ALL]`, `no-new-privileges: true`).

---

## 🧪 Testler ve Doğrulama

Tüm testler yerel ortamda sahte/geçici (ephemeral) sertifikalarla uçtan uca çalıştırılabilir:

```bash
# Tüm test paketini çalıştırın
go test -v ./...

# Yarış durumu (race detector) kontrolü ile test edin
go test -race ./...

# Statik kod analizi yapın
go vet ./...
```

Daha ayrıntılı doğrulama geçmişi ve güvenlik inceleme kayıtları için [docs/VERIFY.md](docs/VERIFY.md) belgesini inceleyebilirsiniz.

---

## 🔒 Güvenlik İlkeleri ve Tavsiyeler

1. **SOCKS Portunu Asla Dış Dünyaya Açmayın**: SOCKS5 protokolü kimlik doğrulamasız kurulur. Bu nedenle yalnızca `127.0.0.1` veya güvenilir iç ağlarda (Docker private bridge, WireGuard/Tailscale VPN) dinletilmelidir.
2. **Güvenlik Duvarı (Firewall)**: Sunucunun QUIC portuna gelen istekler için olası saldırılara karşı güvenlik duvarı hız sınırlandırması (rate limiting) uygulanması önerilir.
3. **Zarif Kapanış**: Agent veya sunucu `Ctrl+C` veya `SIGTERM` sinyali aldığında açık tünelleri ve soketleri temizleyerek zarif şekilde kapanır.
