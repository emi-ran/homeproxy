# Panel doğrulaması — 2026-10-05

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
