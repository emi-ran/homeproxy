# Android geliştirme doğrulaması

2026-10-05, Windows geliştirme makinesi.

- ADB: bağlı arm64 telefon, Android 16 / API 36.
- Flutter stable 3.47.6 yerel araç dizinine kuruldu; global PATH değiştirilmedi.
- `go test ./... -timeout 60s`: geçti, proxy paketi 24.239s.
- `go test ./internal/proxy -run TestMobile -v -timeout 30s`: yaşam döngüsü,
  tekrar başlatma reddi, geçersiz pin ve yanlış sertifika pini testleri geçti.
- `go vet ./...`: geçti.
- `flutter analyze`: geçti.
- `flutter test`: boş bağlantı ayarları form testi geçti.
- `go test -race ...`: çalışmadı; mevcut ortamda CGO kapalı. Race doğrulanmadı.

APK derleme/kurulum ve fiziksel cihaz tünel testi henüz doğrulanmadı.
İlk APK derlemesi NDK kurulumu tamamlanmadan çalıştı; eksik
`source.properties` nedeniyle başarısız oldu. NDK 28.2.13676358 ve Android SDK
Platform 36 kuruldu. Android arm64 `gomobile bind` AAR derlemesi geçti.
`flutter build apk --debug --target-platform android-arm64`: geçti (77.9s).
İlk ADB kurulum denemesi `INSTALL_FAILED_USER_RESTRICTED: Install canceled by user`
ile reddedildi. Kullanıcı onayı sonrası `adb install -r` başarılı oldu.
`am start` mevcut açık activity'ye intent teslim etti; `pidof` çalışan uygulama
sürecini doğruladı. Görsel ekran ve gerçek TCP/UDP tünel testi henüz yapılmadı.
Production deploy, Mori ayarı, telefon ağ/pil ayarı değişikliği yapılmadı.

## İsteğe bağlı insecure modu

Bildirim animasyonlu Android upload ikonu yerine sabit vector ikon kullanıyor;
yalnız durum değişince güncelleniyor. Bu değişiklik APK olarak derlendi; son
bidirim değişikliğinin telefona kurulması henüz doğrulanmadı.

Kullanıcı isteğiyle varsayılanı kapalı TLS doğrulamasını atlama seçeneği eklendi.
UI risk onayı ister; pin yalnız bu açık seçenekle devre dışı kalır. Go mobil
testleri pinli bağlantı, yanlış pin reddi ve açık insecure bağlantıyı doğruladı.
Flutter analyze/test, arm64 AAR ve debug APK build geçti. Güncel APK ADB ile
kuruldu ve activity başlatıldı. Canlı sunucu bağlantısı doğrulanmadı.
