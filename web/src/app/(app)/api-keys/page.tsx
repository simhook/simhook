"use client";

import { useState } from "react";
import { MoreHorizontal, Plus } from "lucide-react";
import { toast } from "sonner";
import type { APIKey } from "@simhook/contracts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Field } from "@/components/field";
import { CopyField } from "@/components/copy-button";
import { EmptyState, PageHeader } from "@/components/page-header";
import { API_URL, errorMessage } from "@/lib/api";
import { absoluteTime, relativeTime } from "@/lib/format";
import { useApiKeyMutations, useApiKeys } from "@/lib/queries";
import { cn } from "@/lib/utils";

const SCOPES = [
  { id: "send", label: "Send messages" },
  { id: "read", label: "Read messages, sends, and stats" },
  { id: "devices", label: "Manage devices and pairing codes" },
  { id: "webhooks", label: "Manage webhooks" },
];

const EXPIRY = [
  { id: "never", label: "Never", days: 0 },
  { id: "30", label: "30 days", days: 30 },
  { id: "90", label: "90 days", days: 90 },
  { id: "365", label: "1 year", days: 365 },
];

function CreateKeyDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { create } = useApiKeyMutations();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(SCOPES.map((s) => s.id));
  const [expiry, setExpiry] = useState("never");
  const [created, setCreated] = useState<string | null>(null);

  const close = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setName("");
      setScopes(SCOPES.map((s) => s.id));
      setExpiry("never");
      setCreated(null);
      create.reset();
    }
  };

  const submit = () => {
    const days = EXPIRY.find((e) => e.id === expiry)?.days ?? 0;
    create.mutate(
      {
        name: name.trim() || undefined,
        scopes,
        expires_at: days ? new Date(Date.now() + days * 86400000).toISOString() : undefined,
      },
      { onSuccess: (res) => setCreated(res.key), onError: (e) => toast.error(errorMessage(e)) },
    );
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Your new API key</DialogTitle>
              <DialogDescription>Copy it now. It is shown once and only its hash is stored.</DialogDescription>
            </DialogHeader>
            <CopyField value={created} secret />
            <p className="text-sm font-medium">Try it</p>
            <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs">{`curl -X POST ${API_URL}/v1/messages \\
  -H "X-Api-Key: ${created}" \\
  -H "Content-Type: application/json" \\
  -d '{"to":["+14155550123"],"body":"Hello from simhook"}'`}</pre>
            <DialogFooter>
              <Button onClick={() => close(false)}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Create an API key</DialogTitle>
              <DialogDescription>Keys act on your behalf. Give each integration its own.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <Field label="Name" htmlFor="key-name">
                <Input id="key-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Production server" maxLength={64} />
              </Field>
              <div className="grid gap-1.5">
                <p className="text-sm font-medium">Permissions</p>
                {SCOPES.map((s) => {
                  const on = scopes.includes(s.id);
                  return (
                    <label key={s.id} className={cn("flex cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-sm", on && "border-primary/50 bg-primary/5")}>
                      <span>
                        <span className="font-mono text-xs">{s.id}</span>
                        <span className="block text-muted-foreground">{s.label}</span>
                      </span>
                      <Switch checked={on} onCheckedChange={(v) => setScopes((cur) => (v ? [...cur, s.id] : cur.filter((x) => x !== s.id)))} />
                    </label>
                  );
                })}
              </div>
              <Field label="Expires" htmlFor="key-expiry">
                <Select value={expiry} onValueChange={(v) => setExpiry(v ?? "never")}>
                  <SelectTrigger id="key-expiry">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {EXPIRY.map((e) => (
                      <SelectItem key={e.id} value={e.id}>
                        {e.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => close(false)}>
                Cancel
              </Button>
              <Button onClick={submit} disabled={create.isPending || scopes.length === 0}>
                {create.isPending ? "Creating…" : "Create key"}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

export default function ApiKeysPage() {
  const [showRevoked, setShowRevoked] = useState(false);
  const keys = useApiKeys(showRevoked);
  const { rename, revoke, remove } = useApiKeyMutations();
  const [creating, setCreating] = useState(false);
  const [renaming, setRenaming] = useState<APIKey | null>(null);
  const [newName, setNewName] = useState("");
  const [confirm, setConfirm] = useState<{ key: APIKey; action: "revoke" | "delete" } | null>(null);
  const list = keys.data?.data ?? [];

  return (
    <>
      <PageHeader
        title="API keys"
        description="Send these in the X-Api-Key header. Anything a key can do, whoever holds it can do."
        actions={
          <Button onClick={() => setCreating(true)} className="gap-1.5">
            <Plus className="size-4" />
            Create key
          </Button>
        }
      />
      <CreateKeyDialog open={creating} onOpenChange={setCreating} />

      <label className="mb-3 flex items-center gap-2 text-sm text-muted-foreground">
        <Switch checked={showRevoked} onCheckedChange={setShowRevoked} /> Show revoked keys
      </label>

      {keys.isPending ? (
        <Skeleton className="h-40" />
      ) : list.length === 0 ? (
        <EmptyState title="No API keys" description="Create one to call the API from your code." action={<Button onClick={() => setCreating(true)}>Create key</Button>} />
      ) : (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Key</TableHead>
                <TableHead>Permissions</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((k) => (
                <TableRow key={k.id} className={cn(k.revoked_at && "text-muted-foreground")}>
                  <TableCell className="font-medium">
                    {k.name}
                    {k.revoked_at ? <Badge variant="outline" className="ml-2">Revoked</Badge> : null}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{k.prefix}…</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {k.scopes.map((s) => (
                        <Badge key={s} variant="secondary" className="font-mono text-[11px]">
                          {s}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap">{k.last_used_at ? relativeTime(k.last_used_at) : "never"}</TableCell>
                  <TableCell className="whitespace-nowrap">{k.expires_at ? absoluteTime(k.expires_at) : "never"}</TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="ghost" size="icon" aria-label="Actions" />}>
                        <MoreHorizontal className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          onClick={() => {
                            setRenaming(k);
                            setNewName(k.name);
                          }}
                        >
                          Rename
                        </DropdownMenuItem>
                        {!k.revoked_at ? <DropdownMenuItem onClick={() => setConfirm({ key: k, action: "revoke" })}>Revoke</DropdownMenuItem> : null}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={() => setConfirm({ key: k, action: "delete" })}>
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={!!renaming} onOpenChange={(o) => !o && setRenaming(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename key</DialogTitle>
          </DialogHeader>
          <Input value={newName} onChange={(e) => setNewName(e.target.value)} maxLength={64} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenaming(null)}>
              Cancel
            </Button>
            <Button
              disabled={!newName.trim() || rename.isPending}
              onClick={() => renaming && rename.mutate({ id: renaming.id, name: newName.trim() }, { onSuccess: () => setRenaming(null), onError: (e) => toast.error(errorMessage(e)) })}
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!confirm} onOpenChange={(o) => !o && setConfirm(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{confirm?.action === "revoke" ? "Revoke this key?" : "Delete this key?"}</DialogTitle>
            <DialogDescription>
              {confirm?.action === "revoke"
                ? "It stops working immediately. Its record stays so you can see when it was last used."
                : "It stops working immediately and its record is removed."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={revoke.isPending || remove.isPending}
              onClick={() => {
                if (!confirm) return;
                const m = confirm.action === "revoke" ? revoke : remove;
                m.mutate(confirm.key.id, { onSuccess: () => setConfirm(null), onError: (e) => toast.error(errorMessage(e)) });
              }}
            >
              {confirm?.action === "revoke" ? "Revoke" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
