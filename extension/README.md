# SantralC-MiniWidget

Her sekmede görünen mini softphone (Chrome MV3 eklentisi, sürüm 0.3.0).
Telefonun kendisi **santral-c panelinde** çalışır (mikrofon orada zaten
çalışıyor). Eklenti, panelin canlı çağrısını **her sekmeye** taşıyan bir
aracı ve ekranın köşesinde duran küçük bir pencereden ibarettir. Bu
pencereden ara, cevapla, sustur, beklet, aktar ve tuşlama (DTMF)
yapabilirsin. Panelle **tek oturum** kullanır, ikisi hep aynı durumu
gösterir.

## Nasıl çalışır

- **İçerik betiği** (`content.js`): her sayfaya küçük pencereyi kapalı bir
  shadow root içinde basar. Sayfanın kendi kodu bu pencereye ulaşamaz ve
  düğmelerine kodla basamaz; düğmeler sadece gerçek tıklamayla çalışır.
  Panel sekmesinde pencere gizlidir; orada betik sadece panelin durumunu
  eklentiye, eklentinin komutlarını panele taşır.
- **Görünürlük kontrolü** (`guard.js`): sayfa, pencerenin dış kutusunu
  saydam yapıp ya da üstüne bir şey koyup seni farkında olmadan bir düğmeye
  bastırmaya çalışabilir. Bu yüzden bir düğme ancak pencere yarım saniyedir
  tam görünürse ve tıkladığın yerde gerçekten pencere varsa çalışır.
- **Arka plan betiği** (`background.js`): sadece aracıdır. Panelin durumunu
  bütün sekmelerdeki pencerelere, pencerelerden gelen komutları panele
  taşır.
- **Panel adresi**: eklenti sadece açılır penceresinde (popup) kayıtlı
  adresteki sekmeyi panel sayar. Bu adresten gelmeyen bir sekme kendini
  panel gibi gösterse de dinlenmez ve komutlar sadece panel sekmesine
  gider. Adres girilmemişse `https://cm.toprakgureli.com` kullanılır.
- Telefon (SIP.js kaydı, WebRTC, mikrofon) **paneldedir**, eklentide değil.
  Bu yüzden eklentinin ayrıca mikrofon izni istemesi gerekmez.

## Kurulum

```bash
cd extension
npm ci
npm run build
```

1. `chrome://extensions` adresini aç, **Developer mode**'u aç, **Load
   unpacked** ile `extension/dist` klasörünü seç.
2. Araç çubuğunda eklentinin simgesine tıkla. **Panel adresi** kutusuna
   panelin adresini yaz (örneğin `https://cm.toprakgureli.com`) ve
   **Kaydet**'e bas. Adres `https://` ile başlamalı; sadece `localhost` ve
   `127.0.0.1` için `http://` olur.
3. **santral-c paneline** bir sekmede giriş yap. Mikrofon iznini panel
   ister, eklenti istemez.
4. Panel sekmesini ve pencereyi görmek istediğin diğer sekmeleri **yenile**.

## Kullanım

Pencere sağ altta çıkar. Boştayken: ön ek (+90 ya da Dahili), numara ve
yeşil Ara düğmesi. Görüşmedeyken: Sustur, Beklet, Tuşlar, Aktar, Kapat.
Gelen çağrıda: Cevapla ya da Reddet. Pencereyi başlığından tutup
sürükleyebilirsin. Panelde yaptığın her şey pencereye, pencerede yaptığın
her şey panele **anında** yansır. Mesaini başlatmadıysan pencere bunu
söyler; mesai açılmadan çağrı gelmez ve arama yapılamaz.

## Test

```bash
cd extension
npm test
```

Arka plan betiğini sahte bir tarayıcı ortamında çalıştırır ve sadece panel
sekmesine inanıldığını, komutların sadece ona gittiğini kontrol eder.
Görünürlük kontrolünü de sahte bir sayfada dener: saydam, gizli, küçültülmüş
ya da üstü örtülmüş bir pencerenin düğmeleri çalışmamalı.

## Notlar

- Telefon panelde çalıştığı için **panelin bir sekmede açık olması**
  gerekir. Panel kapanınca pencere on saniye kadar sonra kaybolur.
- Pencere `chrome://`, mağaza, yeni sekme ve PDF gibi sayfalarda çıkmaz;
  panel sekmesinde de gizlidir.
- Tema, işletim sisteminin açık ya da koyu tercihine uyar.
