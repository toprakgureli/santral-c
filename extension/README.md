# SantralC-MiniWidget

Her sekmede görünen mini softphone (Chrome MV3 eklentisi). Tek bir SIP kaydı
tarayıcı genelinde çalışır (offscreen belge), her sekmeye yüzen bir widget
enjekte edilir. Ara / cevapla / sustur / beklet / aktar / tuş takımı.

## Mimari

- **offscreen belge** (`offscreen.html` + `offscreen.js`): SIP.js ile
  Bulutsantralim'e WSS üzerinden kaydolur, WebRTC medyayı ve tek kaydı tutar.
- **service worker** (`background.js`): offscreen belgeyi ayakta tutar,
  widget'lar ile offscreen arasında mesaj taşır, durumu tüm sekmelere yayar.
- **content script** (`content.js`): her sayfaya shadow-DOM içinde yüzen
  widget'ı basar; komutları gönderir, durumu render eder. Tema OS'in
  açık/koyu tercihini izler.
- **popup** (`popup.html`): SIP bilgilerini (dahili, parola, WSS, domain,
  STUN) saklar ve mikrofon iznini verir.

## Kurulum

```bash
cd extension
npm install
npm run build
```

Sonra Chrome'da:

1. `chrome://extensions` → sağ üstten **Developer mode** açık.
2. **Load unpacked** → `extension/dist` klasörünü seç.
3. Eklenti simgesine tıkla → **Ayarlar**: Dahili (ör. 1014), SIP parola,
   WSS (`wss://api.bulutsantralim.com:7443`), domain
   (`ersinhacioglu.bulutsantralim.com`), STUN (`stun:194.49.126.36:7443`) →
   **Kaydet ve Bağlan** → **Mikrofon İznini Ver**.

SIP parolasını panelden (`/api/v1/sip/credentials`) ya da OIM'den alabilirsin.

## Kullanım

- Widget sağ altta, her sekmede. Boşta: prefix (+90 / Dahili) + numara + yeşil
  Ara. Görüşmede: Sustur, Beklet, Tuşlar (DTMF), Aktar, Kapat. Gelen çağrıda:
  Cevapla / Reddet. Başlıktan sürükle, "—" ile gizle.

## Önemli notlar (ilk sürüm)

- **Tek kayıt:** Eklenti tek SIP kaydı tutar (tüm sekmeler için). Aynı anda
  **web paneli de** aynı dahiliyle açıksa kayıt çakışır. Aynı dahili için
  ikisinden **yalnızca birini** kullan.
- **Mikrofon:** MV3 offscreen belgede mikrofon, önce popup'tan izinle
  verilmeli. Seste sorun olursa "Mikrofon İznini Ver"i tekrar dene.
- **Tema:** Widget OS açık/koyu tercihini izler (eklenti, panelin tema
  ayarını cross-origin okuyamaz). İstenirse panelden senkron eklenebilir.
- **Aktarma:** Bu sürümde numara yazarak (dahili/kuyruk). Açılır listeler
  sonraki adımda (backend'den önbellekli çekilir).
- **Güvenlik:** SIP bilgisi `chrome.storage.local`'da saklanır (yerel).
- Bu ilk sürüm canlı ortamda test edilip ince ayar gerektirebilir.
