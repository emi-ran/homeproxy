# HomeProxy hızlı ayarlar düğmesi

## Kullanım

1. Uygulamada bağlantı bilgilerini girip Başlat'a basın. Ayarlar mevcut Android
   Keystore şifreli kaydına yazılır. Önce bu kayıt gerekir.
2. Telefonun hızlı ayarlar panelini açın, Düzenle/kalem düğmesine basın.
3. HomeProxy düğmesini kullanılan düğmeler arasına sürükleyin.
4. HomeProxy'ye dokunun: kayıtlı ayarlarla bağlanır. Yeniden dokunun: durur.
   Bağlanırken veya yeniden deneme beklerken de durdurabilirsiniz.

Yalnız Go tüneli kimlik doğrulamasından sonra `Bağlı` bildirdiğinde düğme aktif
olur. Bağlanıyor, bağlantı kesildi/yeniden deneme, hata ve durduruldu durumları
pasiftir. Android 10+ alt yazısı durumu gösterir. Uygulama ve bildirimden
başlatma/durdurma aynı servisi kullanır; Go durum callback'i açık paneli
beklemeden günceller. Panel yeniden açıldığında mevcut servis durumu okunur.
Süreç ölürse durum belleği sıfırlanır; otomatik bağlantı veya yeniden başlatma
etkin değildir. Kilitli telefonda değiştirmek için kilit açma istenir.

Eksik/okunamayan kayıt uygulamayı açar; kayıt silinmez, hassas bilgiler hata
mesajına yazılmaz. Insecure tercihi yalnız daha önce kullanıcı tarafından
kaydedilmişse kullanılır; düğme bu seçeneği kendiliğinden açmaz.

Aktif düğmenin gerçek beyaz/vurgulu rengi Android/OEM tema tarafından seçilir.
Uygulama sistem rengini zorlamaz; ekran görüntüsündeki birebir renk garanti değil.

## Bu değişiklik için doğrulama sınırı

Kullanıcı tercihi: tam/soğuk Android derlemeleri gerçek sürümler için saklanır.
Bu işte yerel Gradle, APK/AAR üretimi, CI, push veya deploy yapılmadı.

SDK gerektirmeyen kontroller:

```sh
python3 scripts/check_android_tile.py
GOMAXPROCS=2 GOFLAGS=-p=1 go test ./... -count=1 -timeout 90s
(cd mobile && GOMAXPROCS=2 GOFLAGS=-p=1 go test ./...)
git diff --check
```

Python kontrolleri manifest, aktif durum koşulu, ayar yükleme, stop ve callback
bağlantısı kaynak sözleşmelerini kontrol eder; Kotlin derleme veya cihaz testi
yerine geçmez. Go testleri gerçek yerel QUIC sunucusunda bağlı callback'ini,
yanlış pin sonrası yeniden deneme callback'ini ve bağlanırken iptali doğrular.

Sonraki izinli sürümde yeni `mobile.StatusListener` arayüzü için AAR yeniden
üretilmeli. APK derlemesi ve cihazda uygulama/düğme/bildirim senkronizasyonu,
hızlı tekrar dokunma, süreç öldürme, panel açma, kilit açma, bozuk kayıt ve
OEM tema kontrolü henüz yapılmadı.
