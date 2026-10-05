# HomeProxy Android agent

Flutter ekranı, Kotlin `specialUse` foreground service ve mevcut Go agent.
VPN profili/root gerekmez; yalnız sunucudan gelen proxy trafiği taşınır.

## Derleme

Flutter stable, Android SDK/NDK, Java 17+ ve mobil modül için Go 1.26 gerekir.
Sunucu kök modülü Go 1.25 olarak kalır. Telefon hedefi Android arm64.

```powershell
cd mobile
go run golang.org/x/mobile/cmd/gomobile bind -target=android/arm64 -androidapi 23 -o homeproxy.aar .
cd ../app
flutter analyze
flutter test
flutter build apk --debug --target-platform android-arm64
adb install -r build/app/outputs/flutter-apk/app-debug.apk
```

`gomobile` ve `gobind` araçları PATH üzerinde bulunmalıdır. Android SDK/NDK
kurulu olmalı; `ANDROID_HOME` SDK konumunu göstermelidir. AAR Git'e eklenmez.
APK geliştirme anahtarıyla imzalanır; mağaza/release dağıtımı değildir.

## Bağlantı

Sunucu adresi, benzersiz agent ID, en az 16 bayt token ve sunucunun leaf
sertifikasının SHA-256 parmak izi girilir. Parmak izi güvenilir bağımsız yoldan
alınmalı; telefon bağlandığı sunucuyu otomatik güvenilir kabul etmez.
Varsayılan sertifika pin kontrolüdür. `TLS doğrulamasını atla (insecure)`
seçeneği kullanıcı risk onayından sonra pin gereksinimini kaldırır. TLS şifrelemesi
kalır ancak sunucu kimliği doğrulanmaz; sahte sunucu token'ı ele geçirebilir.
Seçenek diske kaydedilmez ve değişiklik sonraki başlatmada uygulanır.

Sunucu otomatik sertifikasını `server-tls.pem` olarak state dizininde saklar.
Docker/Dokploy için `/var/lib/homeproxy` kalıcı volume'u korunmalıdır. Güvenilir
sunucu logundaki `server TLS certificate SHA-256:` değerini uygulamaya girin.
Volume korunursa restart'ta parmak izi değişmez. İlk eski sürümden geçişte yeni
kalıcı kimlik oluşur. Production mount/deploy otomatik yapılmaz.

Başlat kullanıcı uygulamadayken servisi açar. Flutter ekranı kapansa da servis
tüneli sahiplenir. Durdur veya bildirimdeki Durdur bağlantıyı kapatır.
Android bildirim izni istenir. Ayarlar/token ilk sürümde diske kaydedilmez;
Bildirim sabit ikon kullanır ve yalnız bağlantı durumu değişince güncellenir.
uygulama süreci ölürse yeniden girilir. Otomatik boot/process restart yoktur.

HyperOS pil ve otomatik başlatma ayarları fiziksel test gerektirir. Foreground
service kesintisiz çalışmayı garanti etmez. Mobil veri için Wi-Fi'yi kullanıcı
kapatır; uygulama ağı zorlamaz. Aynı Wi-Fi dış IP kotasını paylaşabilir.
`specialUse` mağaza yayını için ayrı policy incelemesi gerektirir.

Port–agent eşlemesi henüz uygulanmadı; mevcut server routing değişmedi.
Android agent mevcut QUIC/TCP/UDP ve public hedef IP korumasını kullanır.
1100 bayt tünel datagram sınırı ve büyük paket düşürme davranışı korunur.
