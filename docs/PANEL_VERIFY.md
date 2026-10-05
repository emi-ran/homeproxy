# Panel doğrulaması — 2026-10-05

Son push öncesi kontrol: `go test ./... -timeout 60s`, mevcut
`TestLocalManagementAuth` içinde Windows Unix socket `io.ReadAll` beklerken
timeout oldu. Önceki tam test başarısı bu son koşunun başarısızlığını geçersiz
kılmaz; kök neden bu tur çözülmedi. Odaklı
`go test ./internal/proxy -run 'TestPanel|TestMobile|TestPersistentCertificate' -timeout 30s`
ve `go vet ./...` geçti.

Agent QUIC RemoteAddr IP gösterimi ve aynı IP uyarısı eklendi; yalnız giriş
sonrası görünür. Loopback QUIC fixture ile IP çıkarma, duplicate IP uyarısı,
bilinmeyen IP ve template render testi eklendi. Harici IP servisine istek yok.
`go test ./internal/proxy -run TestPanel -v -timeout 30s` ve `go vet ./...`
bu değişikliklerle geçti.

Görsel güncelleme: nötr giriş başlığı/sekmesi, koyu tema, responsive form,
cihaz rozetleri ve düzenli port tablosu eklendi. `TestPanelAuthRoutes` geçti;
giriş HTML'inde ürün adı olmadığını da kontrol eder. Giriş ekranı yerel HTML
önizlemesiyle Playwright'ta açıldı ve screenshot incelendi. Authenticated panel
görseli bu değişiklikte browser üzerinden henüz doğrulanmadı.

- `go test ./... -timeout 60s`: geçti (22.867s).
- Panel testi: yanlış şifre/yetkisiz istek/CSRF reddi, Secure HttpOnly cookie,
  route kaydı, çevrimdışı agent'ta fail-closed, agent seçimi ve logout doğrulandı.
- `go vet ./...` ve `go build ./cmd/homeproxy`: geçti.
- Docker build denenip engellendi: Docker Desktop Linux engine çalışmıyor.
- Browser panel testi, container runtime, Dokploy deploy ve gerçek Mori
  port–agent TCP/UDP testi henüz yapılmadı.

Deploy öncesi Dokploy Environment'a `HOMEPROXY_PANEL_PASSWORD` (en az 16 bayt),
Mounts'a `/var/lib/homeproxy` kalıcı volume, Domain'e HTTPS ve container port
3000 eklenmeli. SOCKS portları hosta publish edilmemeli.
