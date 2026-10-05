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

Fiziksel cihazda gerçek TCP/UDP tünel testi henüz doğrulanmadı.
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

Kullanıcı isteğiyle varsayılanı kapalı TLS doğrulamasını atlama seçeneği eklendi.
UI risk onayı ister; pin yalnız bu açık seçenekle devre dışı kalır. Go mobil
testleri pinli bağlantı, yanlış pin reddi ve açık insecure bağlantıyı doğruladı.
Flutter analyze/test, arm64 AAR ve debug APK build geçti. Güncel APK ADB ile
kuruldu ve activity başlatıldı. Canlı sunucu bağlantısı doğrulanmadı.

## Kalıcı mobil ayarlar

Sunucu, ID, token, parmak izi ve insecure tercihi Android Keystore AES-GCM ile
şifreli kaydedilir. Flutter analiz ve iki widget testi geçti (boş form ve
kayıtlı ayarların otomatik bağlantı açmadan yüklenmesi). Debug APK derlendi ve
ADB güncelleme kurulumu başarılı oldu. Fiziksel cihazda gerçek kaydet/yeniden
aç/güncelleme sonrası geri yükleme henüz doğrulanmadı. Kayıt hataları kullanıcıya
bildirilir; bozuk kayıt sessiz silinmez.
Başlat ayarları otomatik kaydeder; ayrı Kaydet düğmesi kaldırıldı. Bu sürümün
Flutter analiz/test, APK build ve ADB güncelleme kurulumu geçti.

Bildirim animasyonlu Android upload ikonu yerine sabit vector ikon kullanıyor;
yalnız durum değişince güncelleniyor. Güncel APK ile telefona kuruldu; cihazda
bildirim animasyonu davranışı ayrıca gözle doğrulanmadı.
