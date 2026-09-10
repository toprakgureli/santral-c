import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Contact } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, ErrorText, Field, Input } from "../components/ui";

export function Contacts() {
  const { user } = useAuth();
  const manage = can(user, "contact.manage");
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState<Contact | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function load() {
    api
      .listContacts({ query: q || undefined, perPage: 50 })
      .then((r) => { setContacts(r.items); setError(null); })
      .catch((e) => { setContacts([]); setError(e instanceof ApiError ? e.message : "Kişiler yüklenemedi."); });
  }
  useEffect(load, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
      <Card
        title="Kişiler"
        actions={
          <div className="flex gap-2">
            <Input placeholder="Ara" value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => e.key === "Enter" && load()} className="w-40" />
            {manage && <Button onClick={() => { setCreating(true); setSelected(null); }}>Yeni</Button>}
          </div>
        }
      >
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        <ul className="divide-y divide-border/60">
          {contacts.map((c) => (
            <li
              key={c.id}
              className="flex cursor-pointer items-center justify-between py-2.5 hover:bg-accent"
              onClick={() => { setSelected(c); setCreating(false); }}
            >
              <div>
                <div className="font-medium text-foreground">{c.name}</div>
                <div className="text-xs text-muted-foreground">{c.company || c.phones[0]?.number || "—"}</div>
              </div>
              <Badge tone="slate">{c.phones.length} numara</Badge>
            </li>
          ))}
          {contacts.length === 0 && <li className="py-6 text-center text-sm text-muted-foreground">Kişi yok.</li>}
        </ul>
      </Card>

      <div>
        {creating ? (
          <CreateContact
            onCreated={(c) => { setCreating(false); setSelected(c); load(); }}
            onCancel={() => setCreating(false)}
          />
        ) : selected ? (
          <ContactDetail contact={selected} manage={manage} onChanged={(c) => { setSelected(c); load(); }} onDeleted={() => { setSelected(null); load(); }} />
        ) : (
          <Card title="Kişi detayı"><p className="text-sm text-muted-foreground">Bir kişi seçin.</p></Card>
        )}
      </div>
    </div>
  );
}

function CreateContact({ onCreated, onCancel }: { onCreated: (c: Contact) => void; onCancel: () => void }) {
  const [name, setName] = useState("");
  const [company, setCompany] = useState("");
  const [email, setEmail] = useState("");
  const [number, setNumber] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const body: Record<string, unknown> = { name, company, email };
      if (number) body.phones = [{ label: "mobile", number, isPrimary: true }];
      onCreated(await api.createContact(body));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Kayıt başarısız.");
    }
  }

  return (
    <Card title="Yeni kişi" actions={<Button variant="ghost" onClick={onCancel}>İptal</Button>}>
      <form className="space-y-3" onSubmit={submit}>
        <Field label="Ad"><Input value={name} onChange={(e) => setName(e.target.value)} required /></Field>
        <Field label="Şirket"><Input value={company} onChange={(e) => setCompany(e.target.value)} /></Field>
        <Field label="E-posta"><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} /></Field>
        <Field label="Telefon"><Input value={number} onChange={(e) => setNumber(e.target.value)} placeholder="0532..." /></Field>
        <ErrorText>{error}</ErrorText>
        <Button type="submit" className="w-full">Kaydet</Button>
      </form>
    </Card>
  );
}

function ContactDetail({ contact, manage, onChanged, onDeleted }: {
  contact: Contact; manage: boolean; onChanged: (c: Contact) => void; onDeleted: () => void;
}) {
  const [newNumber, setNewNumber] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function addPhone() {
    setError(null);
    try {
      onChanged(await api.addContactPhone(contact.id, { label: "mobile", number: newNumber }));
      setNewNumber("");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Eklenemedi.");
    }
  }
  async function removePhone(phoneId: number) {
    await api.removeContactPhone(contact.id, phoneId).catch(() => undefined);
    onChanged(await api.getContact(contact.id));
  }
  async function remove() {
    await api.deleteContact(contact.id).catch(() => undefined);
    onDeleted();
  }

  return (
    <Card
      title={contact.name}
      actions={manage ? <Button variant="danger" onClick={remove}>Sil</Button> : undefined}
    >
      <dl className="space-y-1 text-sm">
        {contact.company && <div className="flex justify-between"><dt className="text-muted-foreground">Şirket</dt><dd>{contact.company}</dd></div>}
        {contact.email && <div className="flex justify-between"><dt className="text-muted-foreground">E-posta</dt><dd>{contact.email}</dd></div>}
      </dl>

      <h3 className="mb-2 mt-4 text-xs font-semibold uppercase text-muted-foreground">Telefonlar</h3>
      <ul className="space-y-1">
        {contact.phones.map((p) => (
          <li key={p.id} className="flex items-center justify-between text-sm">
            <span>{p.number} <span className="text-muted-foreground">({p.label})</span> {p.isPrimary && <Badge tone="blue">birincil</Badge>}</span>
            {manage && <Button variant="ghost" onClick={() => removePhone(p.id)}>Kaldır</Button>}
          </li>
        ))}
        {contact.phones.length === 0 && <li className="text-sm text-muted-foreground">Numara yok.</li>}
      </ul>

      {manage && (
        <div className="mt-3 flex gap-2">
          <Input placeholder="Yeni numara" value={newNumber} onChange={(e) => setNewNumber(e.target.value)} />
          <Button onClick={addPhone} disabled={!newNumber}>Ekle</Button>
        </div>
      )}
      <ErrorText>{error}</ErrorText>
    </Card>
  );
}
