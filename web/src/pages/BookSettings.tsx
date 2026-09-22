import { Trash2 } from 'lucide-solid';
import { createEffect, createResource, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { CfClass, Role } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input, Select } from '~/components/ui/input';
import { Badge, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate, today } from '~/lib/dates';
import { parseAmount } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const ROLES: Role[] = ['owner', 'editor', 'viewer'];

export default function BookSettings() {
  const { t, te, fieldErrors } = useI18n();
  const { user, currencies } = useSession();
  const book = useBook();

  // ---- general ----
  const [name, setName] = createSignal('');
  const [lockDate, setLockDate] = createSignal('');
  const [cf, setCf] = createSignal<CfClass>('operating');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  createEffect(on(book.book, (b) => {
    if (!b) return;
    setName(b.name);
    setLockDate(b.lock_date ?? '');
    setCf(b.interest_dividend_cf_class);
  }));
  const saveGeneral = async (e: Event) => {
    e.preventDefault();
    try {
      await api.updateBook(book.id(), { name: name(), lock_date: lockDate() || null, interest_dividend_cf_class: cf() });
      book.refetchBook();
      setErrors({});
      toast.success(t('common.saved'));
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };

  // ---- members ----
  const [members, { refetch: refetchMembers }] = createResource(book.id, (id) => api.members(id));
  const [newUser, setNewUser] = createSignal('');
  const [newRole, setNewRole] = createSignal<Role>('editor');
  const run = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      refetchMembers();
    } catch (err) {
      toast.error(te(err));
    }
  };

  // ---- rates ----
  const [prices, { refetch: refetchPrices }] = createResource(book.id, (id) => api.prices(id));
  const [rc, setRc] = createSignal('USD');
  const [rq, setRq] = createSignal('');
  const [rd, setRd] = createSignal(today());
  const [rv, setRv] = createSignal('');
  createEffect(on(book.book, (b) => b && !rq() && setRq(b.base_currency)));
  const addRate = async (e: Event) => {
    e.preventDefault();
    const rate = parseAmount(rv());
    if (!rate) return;
    try {
      await api.addPrice(book.id(), { commodity: rc(), quote: rq(), date: rd(), rate });
      setRv('');
      refetchPrices();
    } catch (err) {
      toast.error(te(err));
    }
  };

  const currencyOptions = () => <For each={currencies() ?? []}>{(c) => <option value={c.code}>{c.code}</option>}</For>;

  return (
    <>
      <PageHeader title={t('book.title')} />
      <div class="grid gap-6">
        <Card>
          <CardHeader><CardTitle>{t('book.general')}</CardTitle></CardHeader>
          <CardContent>
            <form class="grid gap-4 sm:grid-cols-2" onSubmit={saveGeneral}>
              <Field label={t('book.name')} error={errors().name}>
                <Input required disabled={!book.isOwner()} value={name()} onInput={(e) => setName(e.currentTarget.value)} />
              </Field>
              <Field label={t('book.base_currency')} hint={t('accounts.commodity_hint')}>
                <Input disabled value={book.book()?.base_currency ?? ''} />
              </Field>
              <Field label={t('book.lock_date')} hint={t('book.lock_date_hint')}>
                <Input type="date" disabled={!book.isOwner()} value={lockDate()} onChange={(e) => setLockDate(e.currentTarget.value)} />
              </Field>
              <Field label={t('book.cf_choice')} hint={t('book.cf_choice_hint')}>
                <Select disabled={!book.isOwner()} value={cf()} onChange={(e) => setCf(e.currentTarget.value as CfClass)}>
                  <option value="operating">{t('cf.operating')}</option>
                  <option value="investing">{t('cf.investing')}</option>
                </Select>
              </Field>
              <Show when={book.isOwner()}>
                <div class="sm:col-span-2"><Button type="submit">{t('common.save')}</Button></div>
              </Show>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>{t('book.members')}</CardTitle></CardHeader>
          <CardContent class="grid gap-4">
            <Table>
              <tbody>
                <For each={members() ?? []}>
                  {(m) => (
                    <tr class={trClass}>
                      <td class={tdClass}>
                        {m.display_name || m.username}
                        <span class="ml-2 text-xs text-muted-foreground">@{m.username}</span>
                        <Show when={m.user_id === user()?.id}><Badge class="ml-2" variant="outline">{t('book.you')}</Badge></Show>
                      </td>
                      <td class={`${tdClass} w-36`}>
                        <Select disabled={!book.isOwner()} value={m.role}
                          onChange={(e) => run(() => api.updateMember(book.id(), m.user_id, e.currentTarget.value as Role))}>
                          <For each={ROLES}>{(r) => <option value={r}>{t(`role.${r}`)}</option>}</For>
                        </Select>
                      </td>
                      <td class={`${tdClass} w-12 text-right`}>
                        <Show when={book.isOwner() || m.user_id === user()?.id}>
                          <Button variant="ghost" size="icon" aria-label={m.user_id === user()?.id ? t('book.leave') : t('common.remove')}
                            onClick={() => confirm(t('book.remove_confirm')) && run(() => api.removeMember(book.id(), m.user_id))}>
                            <Trash2 />
                          </Button>
                        </Show>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </Table>
            <Show when={book.isOwner()}>
              <form class="flex flex-wrap items-end gap-2" onSubmit={(e) => {
                e.preventDefault();
                run(async () => { await api.addMember(book.id(), newUser(), newRole()); setNewUser(''); });
              }}>
                <Field label={t('book.username')} class="min-w-40 flex-1">
                  <Input required value={newUser()} onInput={(e) => setNewUser(e.currentTarget.value)} />
                </Field>
                <Field label={t('book.role')} class="w-36">
                  <Select value={newRole()} onChange={(e) => setNewRole(e.currentTarget.value as Role)}>
                    <For each={ROLES}>{(r) => <option value={r}>{t(`role.${r}`)}</option>}</For>
                  </Select>
                </Field>
                <Button type="submit">{t('book.add_member')}</Button>
              </form>
            </Show>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('book.rates')}</CardTitle>
            <CardDescription>{t('book.rates_hint')}</CardDescription>
          </CardHeader>
          <CardContent class="grid gap-4">
            <Show when={book.canEdit()}>
              <form class="grid grid-cols-2 items-end gap-2 sm:grid-cols-[6rem_6rem_10rem_1fr_auto]" onSubmit={addRate}>
                <Select aria-label="from" value={rc()} onChange={(e) => setRc(e.currentTarget.value)}>{currencyOptions()}</Select>
                <Select aria-label="to" value={rq()} onChange={(e) => setRq(e.currentTarget.value)}>{currencyOptions()}</Select>
                <Input type="date" value={rd()} onChange={(e) => setRd(e.currentTarget.value)} />
                <MoneyInput placeholder={t('book.rate')} value={rv()} onInput={(e) => setRv(e.currentTarget.value)} />
                <Button type="submit">{t('book.add_rate')}</Button>
              </form>
            </Show>
            <Show when={(prices() ?? []).length} fallback={<p class="text-sm text-muted-foreground">{t('book.no_rates')}</p>}>
              <Table>
                <thead>
                  <tr class="border-b">
                    <th class={thClass}>{t('transactions.date')}</th>
                    <th class={thClass}>{t('transactions.currency')}</th>
                    <th class={`${thClass} text-right`}>{t('book.rate')}</th>
                    <th class={thClass}>{t('book.source')}</th>
                    <th class={thClass} />
                  </tr>
                </thead>
                <tbody>
                  <For each={prices() ?? []}>
                    {(p) => (
                      <tr class={trClass}>
                        <td class={`${tdClass} tabular-nums`}>{formatDate(p.date, user()?.date_format ?? 'YYYY-MM-DD')}</td>
                        <td class={tdClass}>{p.commodity} → {p.quote}</td>
                        <td class={`${tdClass} text-right tabular-nums`}>{p.rate}</td>
                        <td class={tdClass}><Badge variant="outline">{p.source}</Badge></td>
                        <td class={`${tdClass} w-12 text-right`}>
                          <Show when={p.source === 'manual' && book.canEdit()}>
                            <Button variant="ghost" size="icon" aria-label={t('common.delete')}
                              onClick={async () => {
                                try { await api.deletePrice(book.id(), p.id); refetchPrices(); } catch (err) { toast.error(te(err)); }
                              }}>
                              <Trash2 />
                            </Button>
                          </Show>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </Table>
            </Show>
          </CardContent>
        </Card>
      </div>
    </>
  );
}
