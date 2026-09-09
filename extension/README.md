# SantralC-MiniWidget

Her sekmede görünen mini softphone (Chrome MV3 eklentisi). Telefonun kendisi
**santral-c panelinde** çalışır (mikrofon orada zaten çalışıyor); eklenti,
panelin canlı çağrısını **her sekmeye** taşıyan bir röle + yüzen widget'tır.
Widget'tan ara / cevapla / sustur / beklet / aktar / DTMF yapabilirsin ve panel
ile **tek oturum, tam senkron**.

## Mimari

- **content script** (`content.js`): her sayfaya shadow-DOM içinde widget basar.
  Panel sekmesinde ise sessiz köprüdür (panelin durumunu worker'a taşır, widget
  komutlarını panele iletir) ve orada widget'ı gizler.
- **service worker** (`background.js`): saf röle. Panelin durumunu tüm sekme
  widget'larına, widget komutlarını panele taşır.
- Telefon (SIP.js kaydı, WebRTC, mikrofon) **panelde**dir — eklentide değil.
  Böylece offscreen/mikrofon derdi yok.

## Kurulum

```bash
cd extension
npm run build
```

1. `chrome://extensions` → **Developer mode** açık → **Load unpacked** →
   `extension/dist` klasörünü seç.
2. **santral-c paneline** (bir sekmede) giriş yap. Ayar girmene veya mikrofon
   izni vermene **gerek yok** — panel hallediyor.
3. Widget'ı görmek istediğin **normal web sitesi** sekmelerini **yenile**.

## Kullanım

Widget sağ altta çıkar. Boşta: prefix (+90 / Dahili) + numara + yeşil Ara.
Görüşmede: Sustur, Beklet, Tuşlar (DTMF), Aktar, Kapat. Gelen çağrı: Cevapla /
Reddet. Başlıktan sürükle, "—" ile gizle. Panelde yaptığın her şey widget'a,
widget'ta yaptığın her şey panele **anında** yansır.

## Notlar

- Telefon panelde çalıştığı için **panelin bir sekmede açık olması** gerekir.
  Panel kapalıysa widget "Panel kapalı" gösterir.
- Widget `chrome://`, mağaza, yeni-sekme, PDF gibi sayfalarda çıkmaz; panel
  sekmesinde gizlidir.
- Tema OS'in açık/koyu tercihini izler.
