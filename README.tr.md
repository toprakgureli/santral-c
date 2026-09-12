# santral-c

[English](README.md) | Türkçe

santral-c, Verimor'un bulut santrali Bulutsantralim'in üstünde kullandığımız
çağrı yöneticisi. Temsilciler paneldeki tarayıcı softphone'u ile çağrı alıp
açıyor, kimin görüşmede kimin molada olduğunu görüyor, arayanı kişi
rehberinde buluyor, eskalasyon kaydı giriyor ve çağrı kayıtlarını
dinliyor. Yöneticiler aynı panele ek olarak kullanıcı, rol ve yetki yönetimi,
giriş güvenliği kayıtları ve denetim izi görüyor.

Bulut santral iyi yaptığı işi yapmaya devam ediyor (SIP trunk, kuyruk,
kayıt). santral-c bunun etrafına temsilciye dönük katmanı ekliyor: durum
takibi, filtrelenebilir çağrı geçmişi, kişiler, eskalasyonlar ve TOTP'li
düzgün bir kimlik modeli.

## Neler var

**Softphone ve durum takibi**

- Bulutsantralim'e WSS üzerinden kayıt olan SIP.js softphone: sustur, beklet,
  aktar, DTMF. Tarayıcı başına tek kayıt, sekmeler arasında paylaşılır.
- Sayfalar arasında seni takip eden yüzen çağrı çubuğu ve aynı çağrı
  kontrollerini her web sitesine taşıyan isteğe bağlı Chrome eklentisi
  (`extension/`, kendi README'si var).
- Temsilci durumu: müsait, mola, backoffice, rahatsız etmeyin. Müsait
  olmayan durumlar santralde DND açar, temsilciye çağrı düşmez. Günlük durum
  toplamları panoda görünür.
- Server-Sent Events ile canlı temsilci listesi, düşerse polling'e geçer.

**Çağrılar**

- Verimor CDR'ından çağrı geçmişi: yön, sonuç, süre ve kayıt dinleme. Tarihe
  göre filtre (varsayılan bugün; dün, son 7 / 30 gün, bu ay, özel aralık),
  yön, telefon numarası, kendi çağrıların veya belirli bir dahili.
- Son çağrılar, arka planda güncel tutulan sıcak bir bellek penceresinden
  gelir; sık kullanılan görünümler anında açılır ve oran sınırlı API her
  sayfa yüklemesinde yorulmaz. Eski tarihler Verimor'un kendi sunucu tarafı
  sorgusuna gider.
- Mevcut filtreyi CSV olarak indirme (`cdr.export`).
- Numaranın göründüğü her yerden tıkla-ara.

**Kişiler ve eskalasyonlar**

- E.164 aramalı kişi rehberi, kişi başına birden fazla numara, görüşme
  sırasında arayan tanıma.
- Yöneticilerin yönettiği eskalasyon kataloğu (kategori ve neden); temsilci
  hattaki arayan için eskalasyon kaydı girer, arama sayfasında bir numaranın
  eskalasyon geçmişi görülür.

**Kimlik ve yönetim**

- E-posta ve şifreyle giriş, TOTP ikinci adım, ilk girişte zorunlu şifre
  değişimi ve isteğe bağlı zorunlu MFA kurulumu (sistem ayarı).
- Kullanıcılar: profil, roller, SIP hesabı (Verimor'dan çekilir ya da elle
  girilir), üretilmiş geçici şifreyle sıfırlama ve göndermeye hazır
  karşılama mesajı, aktif / pasif.
- Modüle göre gruplanmış yetki kataloğuyla roller. Yeni rol için mevcut
  birini kopyala. Yönetici kendinde olmayan bir yetkiyi veremez.
- Giriş denemeleri, kaldırılabilir IP banları ve her yetkili işlemin denetim
  izi (kim, ne, ne zaman, hangi IP'den).

## Teknoloji

- **Backend**: Go 1.26, Fiber, GORM, PostgreSQL, Redis, goose migration.
  argon2id şifre hash'i, hash'lenmiş refresh oturumlu JWT, TOTP gizli
  anahtarları şifreli saklanır. Uber Go stil rehberine göre yazıldı.
- **Frontend**: React 18, TypeScript, Vite, Tailwind v4, SIP.js.
- **Telefon**: Bulutsantralim REST API (CDR, kullanıcı durumları, kuyruklar,
  çağrı başlatma) ve softphone için WebRTC geçidi.

`DESIGN.md`, planın kendi Asterisk'imizi çalıştırmak olduğu dönemden kalma
tasarım notu. Kimlik ve veri modeli bölümleri hâlâ geçerli; medya motoru ilk
sürümden önce bulut santralle değiştirildi.

## Yerelde çalıştırma

Docker, Go 1.26 ve Node 20 veya üstü gerekir.

1. PostgreSQL ve Redis'i başlat:

   ```bash
   docker compose up -d
   ```

2. `config.example.yml` dosyasını `config.yml` olarak kopyala ve gizli
   değerleri doldur. `bulutsantralim` bloğuna API anahtarı (OİM, Bulut
   Santralım, Santral Ayarlarım), SIP alan adı ve WSS adresi gerekir.
   `config.yml` gitignore'da.

3. Backend'i çalıştır. İlk açılışta migration'ları uygular, yetki kataloğunu,
   sistem rollerini ve sahip hesabını ekler:

   ```bash
   cd backend
   go run ./cmd/santral -config ../config.yml
   ```

4. Frontend'i çalıştır. Geliştirme sunucusu `/api`'yi backend'e yönlendirir;
   `app.port` değiştiyse `VITE_API_TARGET` ile hedefi ver:

   ```bash
   cd frontend
   npm install
   npm run dev
   ```

`config.yml`'deki sahip bilgileriyle giriş yap. Yeni şifre belirlemen,
`security.requireMFA` açıksa TOTP kurman istenir.

Sonra temsilcileri **Kullanıcılar**'dan oluştur, dahililerini gir ve SIP
şifresini "Verimor'dan çek" ile al (dahilinin OİM'de bir personele bağlı
olması gerekir). Her temsilci için geçici şifre ve mesaja yapıştırılacak
karşılama metni hazır gelir.

Push etmeden önce:

```bash
cd backend && go build ./... && go vet ./... && go test ./...
cd frontend && npm run build
```

## Yayına alma

Canlı ortam tek bir Ubuntu sunucuda: nginx Vite çıktısını sunar ve `/api`'yi
systemd altındaki Go binary'sine geçirir, PostgreSQL ve Redis aynı makinede,
önde Cloudflare. Adım adım anlatım `DEPLOY.md`'de. Güncelleme sunucuda tek
komut, sudo yetkili bir kullanıcıyla:

```bash
cd /opt/santral-c && bash deploy/deploy.sh
```

Derleme git SHA'sı ve saatle damgalanır; kenar çubuğunda görünür, çalışan
sürümün güncel olup olmadığı bir bakışta anlaşılır.

## Kilitlenen sahip hesabını kurtarma

`backend/cmd/resetpw` bir kullanıcının şifresini doğrudan veritabanında
değiştirir, MFA'yı temizler ve kilidi açar. E-posta yoksa görünmez yönetici
olarak oluşturur:

```bash
cd backend
go run ./cmd/resetpw -config ../config.yml -email owner@example.com -password 'NewPass123!'
```

## Verimor API'si hakkında bilinmesi gerekenler

Bunlar bize zaman kaybettirdi, o yüzden buraya yazıldı.

- `/cdrs` tarih aralığı verilmezse yalnızca bugünün çağrılarını döner; ilk
  sayfadan sonrası zaman aşımına düşecek kadar yavaş. Backend bu yüzden 1.
  sayfadan kayan bir pencere biriktirip bellekte filtreler; geçmiş tarihler
  `start_stamp_from` ve `start_stamp_to` ile sorgulanır, eksiksizdir ama
  sayfa başına on beş saniye civarı sürer.
- `number` filtresi kısa dahililerde çalışmaz, neredeyse tüm santrali döner.
  Dahili filtresi `1014 (9021...)` ve `9021... (1008)` taraf biçimleri
  eşleştirilerek uygulama içinde yapılır.
- `/user_statuses` yirmi saniye kadar sürer ve dakikada birkaç çağrıyla
  sınırlı. Arka planda sorgulanır; panel doğrudan çağırmaz.
- Art arda istekler 429 döner. Poller çağrılarını aralıklı atar, kısıtlama
  yiyince geri çekilir.
- SIP şifrelerini veren webphone sayfası yalnızca Türkiye IP'lerine sunulur.
  Yurt dışındaki sunucudan OİM giriş sayfası gelir; "Verimor'dan çek"
  Türkiye'deki bir makineden çalışır, aksi halde şifre elle girilir.

## Depo düzeni

```
backend/     Go servisi: cmd/santral (sunucu), cmd/resetpw, internal/* modülleri, migration'lar
frontend/    React panel ve softphone
extension/   Panelin çağrısını her sekmeye taşıyan Chrome MV3 mini widget
deploy/      deploy.sh, systemd birimi, nginx site tanımı
```

## Lisans

Özel, şirket içi proje.
