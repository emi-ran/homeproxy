# Telefon agent ve sabit proxy portları

Tarih: 2026-10-05
Durum: Fikir kaydı; henüz uygulanmadı.

## Amaç

HomeProxy'nin ikinci çıkış cihazı ikinci PC değil, telefon olacak. Önce
yarım kalan telefon uygulaması ele alınacak. Telefonun tünele nasıl bağlanacağı
ve arka planda nasıl çalışacağı daha sonra değerlendirilecek.

Kullanıcı düzeltmesi: yarım kalan Flutter uygulaması yok; önceki çalışma bir
mimari sohbetiydi. Tailscale denemesi ayrıydı. HomeProxy telefon agent için VPN
profili gerekmiyor; Flutter UI + Kotlin foreground service + Go core seçildi.
İlk Android kaynakları `app/`, Go köprüsü `mobile/` altında geliştiriliyor.

## Seçilen basit yönlendirme fikri

Mori'de otomatik hesap/kapasite dağıtımı şimdilik yapılmayacak. Tek HomeProxy
sunucusunda ayrı SOCKS portları belirli agent ID'lerine sabitlenecek:

| Örnek SOCKS adresi | Agent ID | Çıkış cihazı |
| --- | --- | --- |
| `homeproxy:1081` | `ev-pc` | Mevcut PC |
| `homeproxy:1082` | `telefon` | Telefon |

Adres, port ve agent ID'leri örnektir; mevcut PC ayarlarını koruyacak kesin
değerler uygulama öncesi belirlenecek. İki agent aynı sunucunun QUIC dinleyicisine
bağlanabilir; ayrı SOCKS portları ayrı QUIC portları gerektirmez.

- Mori'de ilk üç hesabın proxy'si PC portu, diğer üç hesabın proxy'si telefon
  portu olarak elle ayarlanacak. Mori kodunda değişiklik hedeflenmiyor.
- Her portun TCP CONNECT ve UDP ASSOCIATE trafiği yalnız atanmış agent'a gidecek.
- Bir cihaz koparsa o porttaki hesaplar bağlantı kaybedecek; diğer cihaza otomatik
  failover yapılmayacak. Böylece dolu çıkış IP'sine ek hesap taşınmayacak.
- Cihaz geri geldiğinde hesapların toparlanması Mori'nin reconnect davranışına
  bağlı; canlı doğrulama gerekecek.
- Sistem üç hesap kotasını otomatik uygulamayacak; dağıtım kullanıcı sorumluluğunda.
- SOCKS portları yalnız güvenilir iç ağda erişilebilir kalacak; internete açılmayacak.

## Kapasite şartı

Kullanıcının belirttiği Growtopia sınırı: dış IP başına aynı anda en fazla üç
cihaz/hesap. Bu sınır burada bağımsız doğrulanmış değildir.

Altı hesap hedefi için PC ve telefonun dış IP'leri farklı olmalı. Telefon PC ile
aynı Wi-Fi/internet çıkışını kullanırsa ayrı cihaz ve port toplam kapasiteyi
artırmaz. Mobil veri ayrı çıkış sağlayabilir; gerçek dış IP kontrol edilmelidir.
Diğer oyun istemcileri de aynı IP'nin kotasını tüketebilir.

## Mevcut durum ve sonraki sıra

Mevcut HomeProxy `-socks` ile tek SOCKS dinleyicisi açıyor; agent seçimi
`internal/proxy/proxy.go` içinde yapılıyor. Port–agent eşlemesi henüz yok.
Önceki konuşmadaki `-socks-agent` parametresi yalnız öneriydi; mevcut CLI'de yok.

1. Mevcut Go agent'ı Android AAR olarak derleyip Flutter/Kotlin uygulamasına bağla.
2. Telefon agent'ın bağlantı, trafik çıkışı ve arka plan çalışma yöntemini belirle;
   VPN profili gerekip gerekmediğine inceleme sonrası karar ver.
3. Telefon agent'ı çalışır hale getirip ayrı dış IP'den TCP/UDP çıkışını doğrula.
4. HomeProxy'ye en küçük sabit port–agent eşlemesini ekle; TCP/UDP ve agent
   kopması testleriyle yanlış cihaza failover olmadığını doğrula.
5. Mori'de hesapları iki porta elle dağıtıp canlı bağlantı ve reconnect kontrolü yap.

Bu kayıt telefon uygulamasının mimarisini seçmez ve geliştirme başlatmaz.
