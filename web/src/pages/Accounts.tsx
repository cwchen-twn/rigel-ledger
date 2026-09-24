import { useSearchParams } from '@solidjs/router';
import { Archive, ArchiveRestore, Ellipsis, Pencil, Plus, Trash2 } from 'lucide-solid';
import { batch, createEffect, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Account, AccountClass, CfClass } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { NewAccountDialog, QUICK_KINDS, type QuickKind } from '~/components/NewAccountDialog';
import { MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { DropdownMenu } from '~/components/ui/dropdown-menu';
import { Checkbox, Field, Input, Select } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { commodityOptions } from '~/lib/options';
import { Badge, Skeleton } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { today } from '~/lib/dates';
import { parseAmount } from '~/lib/money';
import { CLASS_ORDER, useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const holdsCommodity = (c: AccountClass) => c === 'asset' || c === 'liability' || c === 'equity';

function AccountDialog(props: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  account: Account | null; // editing
  parent: Account | null; // new child of
  cls: AccountClass; // new top-level of
}) {
  const { t, te, fieldErrors } = useI18n();
  const { commodities, kind } = useSession();
  const book = useBook();
  const base = () => book.book()?.base_currency ?? 'USD';

  const [name, setName] = createSignal('');
  const [code, setCode] = createSignal('');
  const [cls, setCls] = createSignal<AccountClass>('asset');
  const [parentId, setParentId] = createSignal<number | null>(null);
  const [commodity, setCommodity] = createSignal('');
  const [isCurrent, setIsCurrent] = createSignal(true);
  const [isCash, setIsCash] = createSignal(false);
  const [cf, setCf] = createSignal<CfClass>('operating');
  const [placeholder, setPlaceholder] = createSignal(false);
  const [opening, setOpening] = createSignal('');
  const [openingBase, setOpeningBase] = createSignal('');
  const [openingDate, setOpeningDate] = createSignal(today());
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  createEffect(
    on(
      () => props.open,
      (open) => {
        if (!open) return;
        const a = props.account;
        const p = props.parent;
        batch(() => {
          setErrors({});
          setName(a?.name ?? '');
          setCode(a?.code ?? '');
          setCls(a?.class ?? p?.class ?? props.cls);
          setParentId(a ? a.parent_id : p?.id ?? null);
          setCommodity(a?.commodity ?? p?.commodity ?? base());
          setIsCurrent(a?.is_current ?? p?.is_current ?? true);
          setIsCash(a?.is_cash ?? false);
          setCf(a?.cf_class ?? p?.cf_class ?? 'operating');
          setPlaceholder(a?.is_placeholder ?? false);
          setOpening('');
          setOpeningBase('');
          setOpeningDate(today());
        });
      },
    ),
  );

  const editing = () => !!props.account;
  const parents = () =>
    (book.accounts() ?? []).filter((a) => a.class === cls() && a.id !== props.account?.id && !a.archived);
  const takesOpening = () => !editing() && !placeholder() && (cls() === 'asset' || cls() === 'liability');

  const save = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    const ob = parseAmount(opening());
    const input = {
      parent_id: parentId(),
      class: cls(),
      name: name().trim() || null,
      code: code().trim() || null,
      commodity: holdsCommodity(cls()) ? commodity() : null,
      is_current: isCurrent(),
      is_cash: cls() === 'asset' && isCash(),
      cf_class: cf(),
      is_placeholder: placeholder(),
      opening_balance:
        takesOpening() && ob
          ? { amount: ob, date: openingDate(), ...(commodity() !== base() && parseAmount(openingBase()) ? { base_amount: parseAmount(openingBase())! } : {}) }
          : null,
    };
    try {
      if (props.account) await api.updateAccount(book.id(), props.account.id, input);
      else await api.createAccount(book.id(), input);
      toast.success(t('common.saved'));
      book.refetchAccounts();
      props.onOpenChange(false);
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={editing() ? t('accounts.edit') : props.parent ? t('accounts.new_child') : t('accounts.new')}
      footer={
        <>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button type="submit" form="account-form" disabled={busy()}>{busy() ? t('common.saving') : t('common.save')}</Button>
        </>
      }
    >
      <form id="account-form" class="grid gap-4" onSubmit={save}>
        <Field label={t('accounts.name')} error={errors().name} hint={props.account?.template_key ? t('accounts.name_hint') : undefined}>
          <Input
            required={!props.account?.template_key}
            placeholder={props.account ? book.name(props.account) : ''}
            value={name()}
            onInput={(e) => setName(e.currentTarget.value)}
          />
        </Field>
        <div class="grid gap-4 sm:grid-cols-2">
          <Field label={t('accounts.class')}>
            <Select value={cls()} disabled={editing() || parentId() !== null} onChange={(e) => setCls(e.currentTarget.value as AccountClass)}>
              <For each={CLASS_ORDER}>{(c) => <option value={c}>{t(`class_one.${c}`)}</option>}</For>
            </Select>
          </Field>
          <Field label={t('accounts.parent')} error={errors().parent_id}>
            <Select value={parentId() ?? ''} onChange={(e) => setParentId(e.currentTarget.value ? Number(e.currentTarget.value) : null)}>
              <option value="">{t('accounts.no_parent')}</option>
              <For each={parents()}>{(a) => <option value={a.id}>{book.path(a.id)}</option>}</For>
            </Select>
          </Field>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <Show when={holdsCommodity(cls())}>
            <Field label={t('accounts.commodity')} hint={t('accounts.commodity_hint')} error={errors().commodity}>
              <SearchSelect options={commodityOptions(commodities())} value={commodity()} disabled={editing()} onChange={(c) => {
                setCommodity(c);
                // Shares are bought and sold as investing activity (IAS 7).
                if (kind(c) === 'security') setCf('investing');
              }} />
            </Field>
          </Show>
          <Field label={t('accounts.code')}>
            <Input value={code()} onInput={(e) => setCode(e.currentTarget.value)} placeholder={t('common.optional')} />
          </Field>
        </div>
        <Field label={t('accounts.cf_class')}>
          <Select value={cf()} onChange={(e) => setCf(e.currentTarget.value as CfClass)}>
            <For each={['operating', 'investing', 'financing'] as CfClass[]}>{(c) => <option value={c}>{t(`cf.${c}`)}</option>}</For>
          </Select>
        </Field>
        <div class="grid gap-2">
          <Show when={cls() === 'asset' || cls() === 'liability'}>
            <Checkbox checked={isCurrent()} onChange={(e) => setIsCurrent(e.currentTarget.checked)} label={t('accounts.is_current')} />
          </Show>
          <Show when={cls() === 'asset'}>
            <Checkbox checked={isCash()} onChange={(e) => setIsCash(e.currentTarget.checked)} label={t('accounts.is_cash')} />
          </Show>
          <Checkbox checked={placeholder()} onChange={(e) => setPlaceholder(e.currentTarget.checked)} label={t('accounts.placeholder')} />
        </div>
        <Show when={takesOpening()}>
          <div class="grid gap-3 rounded-lg border p-3">
            <Field label={t('accounts.opening_balance')} hint={t('accounts.opening_hint')} error={errors().opening_balance}>
              <div class="flex items-center gap-2">
                <MoneyInput value={opening()} onInput={(e) => setOpening(e.currentTarget.value)} placeholder={t('common.optional')} />
                <span class="w-12 text-sm text-muted-foreground">{commodity()}</span>
              </div>
            </Field>
            <div class="grid gap-3 sm:grid-cols-2">
              <Field label={t('accounts.opening_date')}>
                <Input type="date" value={openingDate()} onChange={(e) => setOpeningDate(e.currentTarget.value)} />
              </Field>
              <Show when={commodity() !== base()}>
                <Field label={`${t('accounts.opening_base')} (${base()})`} error={errors()['lines[0].base_amount']}>
                  <MoneyInput value={openingBase()} onInput={(e) => setOpeningBase(e.currentTarget.value)} placeholder={t('common.optional')} />
                </Field>
              </Show>
            </div>
          </div>
        </Show>
      </form>
    </Dialog>
  );
}

export default function Accounts() {
  const [search, setSearch] = useSearchParams();
  const wanted = () => (typeof search.new === 'string' && QUICK_KINDS.includes(search.new as QuickKind) ? (search.new as QuickKind) : undefined);
  const [quick, setQuick] = createSignal(!!wanted());
  const { t, te } = useI18n();
  const book = useBook();
  const [showArchived, setShowArchived] = createSignal(false);
  const [dialog, setDialog] = createSignal<{ account: Account | null; parent: Account | null; cls: AccountClass } | null>(null);

  const archive = async (a: Account, archived: boolean) => {
    try {
      await api.archiveAccount(book.id(), a.id, archived);
      book.refetchAccounts();
    } catch (err) {
      toast.error(te(err));
    }
  };
  const remove = async (a: Account) => {
    if (!confirm(t('accounts.delete_confirm'))) return;
    try {
      await api.deleteAccount(book.id(), a.id);
      toast.success(t('common.deleted'));
      book.refetchAccounts();
    } catch (err) {
      toast.error(te(err));
    }
  };

  const kids = (parent: number | null, cls: AccountClass) =>
    (book.children().get(parent) ?? []).filter((a) => a.class === cls && (showArchived() || !a.archived));

  function Row(props: { account: Account; depth: number }) {
    const a = () => props.account;
    return (
      <>
        <div class={cn('flex items-center gap-2 border-b py-2 text-sm last:border-b-0', a().archived && 'opacity-60')}
          style={{ 'padding-left': `${props.depth * 1.25}rem` }}>
          <span class={cn('min-w-0 flex-1 truncate', a().is_placeholder && 'font-medium')}>
            <Show when={a().code}><span class="mr-2 text-muted-foreground tabular-nums">{a().code}</span></Show>
            {book.name(a())}
          </span>
          <Show when={a().is_cash}><Badge variant="outline">{t('accounts.cash')}</Badge></Show>
          <Show when={a().is_placeholder}><Badge variant="outline">{t('accounts.group')}</Badge></Show>
          <Show when={a().archived}><Badge>{t('accounts.archived')}</Badge></Show>
          <Show when={a().commodity}><span class="w-10 text-right text-xs text-muted-foreground">{a().commodity}</span></Show>
          <Show when={book.canEdit()}>
            <DropdownMenu
              label={t('common.actions')}
              triggerClass="rounded-md p-1 text-muted-foreground hover:bg-accent"
              trigger={<Ellipsis class="size-4" />}
              items={[
                { label: t('common.edit'), icon: <Pencil />, onSelect: () => setDialog({ account: a(), parent: null, cls: a().class }) },
                { label: t('accounts.new_child'), icon: <Plus />, onSelect: () => setDialog({ account: null, parent: a(), cls: a().class }) },
                a().archived
                  ? { label: t('accounts.unarchive'), icon: <ArchiveRestore />, onSelect: () => archive(a(), false) }
                  : { label: t('accounts.archive'), icon: <Archive />, onSelect: () => archive(a(), true) },
                { label: t('common.delete'), icon: <Trash2 />, destructive: true, separatorBefore: true, onSelect: () => remove(a()) },
              ]}
            />
          </Show>
        </div>
        <For each={kids(a().id, a().class)}>{(k) => <Row account={k} depth={props.depth + 1} />}</For>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title={t('accounts.title')}
        description={t('accounts.page_hint')}
        actions={
          <>
            <Checkbox checked={showArchived()} onChange={(e) => setShowArchived(e.currentTarget.checked)} label={t('accounts.show_archived')} />
            <Show when={book.canEdit()}>
              <Button onClick={() => setQuick(true)}><Plus /> {t('accounts.quick_title')}</Button>
            </Show>
          </>
        }
      />
      <Show when={search.welcome === '1'}>
        <div class="mb-4 rounded-lg border border-primary/30 bg-accent/40 p-4 text-sm">
          <p class="font-medium">{t('accounts.welcome_title')}</p>
          <p class="text-muted-foreground">{t('accounts.welcome_hint')}</p>
        </div>
      </Show>
      <NewAccountDialog
        open={quick()}
        kind={wanted()}
        onOpenChange={(o) => {
          setQuick(o);
          if (!o && search.new) setSearch({ new: undefined });
        }}
      />
      <Show when={book.accounts()} fallback={<Skeleton class="h-96 w-full" />}>
        <div class="grid gap-4 lg:grid-cols-2">
          <For each={CLASS_ORDER}>
            {(cls) => (
              <Card class="gap-2 py-4">
                <CardHeader class="flex flex-row items-center justify-between px-4">
                  <CardTitle class="text-base">{t(`class.${cls}`)}</CardTitle>
                  <Show when={book.canEdit()}>
                    <Button variant="ghost" size="sm" onClick={() => setDialog({ account: null, parent: null, cls })}>
                      <Plus /> {t('common.add')}
                    </Button>
                  </Show>
                </CardHeader>
                <CardContent class="px-4">
                  <For each={kids(null, cls)}>{(a) => <Row account={a} depth={0} />}</For>
                </CardContent>
              </Card>
            )}
          </For>
        </div>
      </Show>
      <AccountDialog
        open={dialog() !== null}
        onOpenChange={(o) => !o && setDialog(null)}
        account={dialog()?.account ?? null}
        parent={dialog()?.parent ?? null}
        cls={dialog()?.cls ?? 'asset'}
      />
    </>
  );
}
