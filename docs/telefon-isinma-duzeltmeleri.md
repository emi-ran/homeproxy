# Telefon ısınması — HomeProxy + Mori düzeltmeleri

Kaynak: kod incelemesi (canı canlı ölçüm yok). Isının tamamını bu düzeltmeler garanti etmez;
MTU ve keepalive israfı kesin, gerisi ihtimali yüksek.

## Durum (ölçülen, sunucu tarafı)

- `homeproxy` pid 1653094: 6s07m uptime, 10m44s CPU, **%2.8**, RSS 3.9 MB
- `Mori` pid 1806778: 3s32m uptime, 10m10s CPU, **%4.7**, RSS 1.5 GB
- Son 10 sn'de her iki tarafta **0 UDP paketi** → canlı ısı ölçümü mümkün olmadı, bulgular koddan.
- Telefon tarafı testi (hot loop birebir taklit): `9.51 µs/pkt` → ~105k pkt/s/çekirdek tavanı;
  4 bot @ 20pps tek çekirdeğin **%0.08'i**. Isı paket hacminden değil; retransmit + keepalive + reconnect churn'dan.

## 1. MTU çatışması → paket düşürme + retransmit (asıl fix)

- ENet varsayılan MTU `HOST_DEFAULT_MTU = 1392` (rusty_enet, `consts.rs:32`).
  Mori hiçbir yerde `set_mtu` çağırmıyor → **1392**.
- HomeProxy tavanı: `maxDatagram = 1100` (`internal/proxy/udp.go:19`) +8 tunnel id,
  -8 byte QUIC header, -10 byte SOCKS5 UDP header (IPv4) →
  **maks geçen ENet payload = 1082 byte**.
- 1082–1392 arası her ENet paketi düşer:
  ```go
  if len(b)+8 > maxDatagram { oversize.Add(1); log.Print("oversize UDP dropped"); return }
  ```
- ENet güvenilir-UDP: düşen paketi **süresiz** tekrar gönderir (üstel backoff). Her deneme yine düşer.
  Her düşüşte `log.Print` → Android logcat yazma. Kalıcı retransmit döngüsü + radyo wake + ısı.
- Dönüş yönü de aynı tavan: `3 + 7 + payload ≤ 1100`.
- quic-go datagram için 16383'e izin veriyor; 1100 keyfi.

**Fix:** HomeProxy `maxDatagram`'ı ≥1400'e çıkar **veya** Mori'de `host.set_mtu(1000)` (Mori çapında,
tek satır; `working_growtopia_proxies.txt` üzerinden diğer botlarda da MTU 1392 görünüyor — sadece
SOCKS5 yola sınırlamak bot başına yanlış MTU seçer).

## 2. QUIC keepalive 5 sn → 30 sn

`internal/proxy/proxy.go`:
```go
var qc = &quic.Config{ MaxIdleTimeout: 20*time.Second, KeepAlivePeriod: 5*time.Second, ... }
```
`qc` hem sunucuda hem telefonda kullanılıyor. 5 sn'de bir QUIC PING → **sıfır trafikte bile**
hücresel/Wi-Fi radyosu hiç uykuya girmiyor (ısı + batarya). Foreground service (`specialUse`)
zaten doze'u engelliyor.

**Fix:** `KeepAlivePeriod: 30*time.Second`, `MaxIdleTimeout` ile uyumlu.

## 3. 60 sn idle → association öldürme

`internal/proxy/mobile.go` → `agentUDP = agentUDPTimeouts(c, q, allow, 60*time.Second, time.Hour)`
Telefonda `u.ReadFromUDP` 60 sn sessizlikte hata → `return` → `defer q.Close()` →
**tüm UDP association'ı kapanır**. Trafik tek yönlü olduğunda tünel kopar; yeniden associate,
yeni QUIC stream. Sürekli churn.

**Fix:** downlink sessizliğinde association'ı kapatma; socket'i canlı tut,
sadece gönderim deadline'ını tazele.

## 4. Reconnect fırtınası (3 sn sabit)

`internal/proxy/mobile.go:80-93` → `runAgentStatus` hata verirse **3 sn** sonra tekrar.
Mobil NAT carrier UDP timeout'u 20 sn'den kısaysa veya paket kaybı varsa bu **sürekli reconnect**:
her denemede tam TLS 1.3 + ECDHE el sıkışması. Isınmayı en çok açıklayan aday —
öncelik #1 ile birlikte test edilmeli.

**Fix:** jitter'lı backoff → 5/10/20/60 sn.

## 5. Telefon GC/procs

`app/android/.../AgentService.kt` + `mobile/bridge.go`: `debug.SetGCPercent` ve `GOMAXPROCS`
sınırı **hiç** ayarlanmamış → Go runtime tüm çekirdekleri kullanıyor, big.LITTLE'da küçük
çekirdekler de uyanık kalıyor.

**Fix:** `debug.SetGCPercent(50)` + `GOMAXPROCS` sınırı (ör. 2–4).

## Öncelik

1. MTU eşle — tek başına yüksek ihtimal yeter (israf kesin + örnekteki devamı da bunu gösteriyor)
2. Keepalive 5→30 sn
3. Idle timeout association'ı öldürmesin
4. Reconnect backoff
5. Telefon GC/procs

Hepsi küçük diff. Deploy yok; commit + test.

## (Ertelendi) Kaba ölçüm — gerekirse

```bash
adb shell dumpsys thermalservice | grep -A3 mThermalStatus   # sıcaklık + sensör tipi
adb shell top -H -p $(adb shell pidof com.homeproxy.homeproxy_agent)
adb shell cat /proc/net/snmp | grep -E '^Udp:'               # RcvbufErrors / retransmit
adb shell dumpsys netstats detail | grep -A5 "iface=wlan0"
```
Ön/son karşılaştırma: önce 5–10 dk baseline, düzeltme, aynı koşulda tekrar.
`dumpsys thermalservice` CPU sensörü çıkarsa CPU; radyo sensörü çıkarsa keepalive teyit eder.

## Not: kalan ısı tabanı

Telefon burada **çıkış (egress)**: Mori'nin tüm Growtopia trafiği telefonun radyosundan geçiyor.
İsraf düzelse de bu rolün belirli bir ısısı kalır (ön servis + sürekli UDP aktarım).
Hedef %0 değil, "deli gibi" → "normal".
