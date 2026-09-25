// ShareDialog shows the team picture and hands it over as a file or to the
// clipboard.

import { useEffect, useState } from "react";
import { Check, Copy, Download } from "lucide-react";
import type { TeamRow } from "@/api/types";
import { Button, Modal } from "@/components/ui";
import { renderTeamImage } from "@/pages/teamShare";

export default function ShareDialog({ open, rows, rangeLabel, scopeLabel, onClose }: { open: boolean; rows: TeamRow[]; rangeLabel: string; scopeLabel: string; onClose: () => void }) {
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) return;
    let live = true;
    setCanvas(null);
    setUrl(null);
    setError(null);
    renderTeamImage(rows, rangeLabel, scopeLabel)
      .then((c) => {
        if (!live) return;
        setCanvas(c);
        setUrl(c.toDataURL("image/png"));
      })
      .catch(() => live && setError("Görsel oluşturulamadı."));
    return () => {
      live = false;
    };
  }, [open, rows, rangeLabel, scopeLabel]);

  const file = `ekip-performansi-${rangeLabel.replace(/[^\d]+/g, "-").replace(/^-|-$/g, "") || "bugun"}.png`;

  const download = () => {
    if (!url) return;
    const a = document.createElement("a");
    a.href = url;
    a.download = file;
    a.click();
  };

  const copy = async () => {
    if (!canvas) return;
    try {
      const blob = await new Promise<Blob | null>((r) => canvas.toBlob(r, "image/png"));
      if (!blob) throw new Error();
      await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setError("Panoya kopyalanamadı, indirmeyi kullan.");
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Performans görseli"
      description="Sıralama, rakamlar ve kişi başı özet. İndir ya da panoya kopyalayıp sohbete yapıştır."
      size="lg"
      footer={
        <>
          {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
          <Button variant="secondary" onClick={copy} disabled={!canvas}>{copied ? <Check className="size-4" /> : <Copy className="size-4" />} {copied ? "Kopyalandı" : "Panoya kopyala"}</Button>
          <Button onClick={download} disabled={!url}><Download className="size-4" /> PNG indir</Button>
        </>
      }
    >
      {url ? (
        <img src={url} alt="Ekip performansı" className="w-full rounded-xl border border-border/60 shadow-sm" />
      ) : (
        <p className="py-10 text-center text-sm text-muted-foreground">{error ?? "Görsel hazırlanıyor..."}</p>
      )}
    </Modal>
  );
}
