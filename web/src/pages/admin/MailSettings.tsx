import { Send } from 'lucide-solid';
import { createMemo, createSignal, For, Show } from 'solid-js';
import { createStore, unwrap } from 'solid-js/store';
import { api } from '~/api/client';
import type { MailDriver, MailSettingsInput, SmtpSecurity, SystemSettings } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Checkbox, Field, Input, Select } from '~/components/ui/input';
import { Badge } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';

const DRIVERS: MailDriver[] = ['smtp', 'log', 'off'];
const SECURITY: SmtpSecurity[] = ['starttls', 'tls', 'none'];

/** Outgoing mail. The password is write-only: it is never sent back. */
export function MailSettings(props: { settings: SystemSettings; onSaved: (s: SystemSettings) => void }) {
  const { t, te, fieldErrors } = useI18n();
  const fromSettings = (s: SystemSettings): MailSettingsInput => ({
    mail_driver: s.mail_driver, smtp_host: s.smtp_host, smtp_port: s.smtp_port, smtp_security: s.smtp_security,
    smtp_user: s.smtp_user, smtp_password: '', clear_smtp_password: false, mail_from: s.mail_from, mail_from_name: s.mail_from_name,
  });
  const s = props.settings;
  const [form, setForm] = createStore<MailSettingsInput>(fromSettings(s));
  // What the server has. The test mail always uses this, never the form.
  const [saved, setSaved] = createSignal<MailSettingsInput>(fromSettings(s));
  const dirty = createMemo(() => {
    const a = saved();
    return (
      !!form.smtp_password || !!form.clear_smtp_password ||
      (['mail_driver', 'smtp_host', 'smtp_port', 'smtp_security', 'smtp_user', 'mail_from', 'mail_from_name'] as const)
        .some((k) => String(form[k] ?? '') !== String(a[k] ?? ''))
    );
  });
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);
  const [passwordSet, setPasswordSet] = createSignal(s.smtp_password_set);

  /** Saves the form; false when the server refused it. */
  const persist = async (quiet = false): Promise<boolean> => {
    setBusy(true);
    try {
      const out = await api.admin.updateMail(structuredClone(unwrap(form)));
      props.onSaved(out);
      setPasswordSet(out.smtp_password_set);
      setForm({ smtp_password: '', clear_smtp_password: false });
      setSaved(fromSettings(out));
      setErrors({});
      if (!quiet) toast.success(t('common.saved'));
      return true;
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const save = async (e: Event) => {
    e.preventDefault();
    await persist();
  };

  // The test uses the saved settings, so unsaved changes are saved first:
  // testing what is on screen while the server still holds something else
  // is how a test "succeeded" into the log driver before.
  const test = async () => {
    if (dirty() && !(await persist(true))) return;
    setBusy(true);
    try {
      const r = await api.admin.testMail();
      if (r.driver === 'smtp') toast.success(t('admin.test_sent_to', { email: r.to }));
      else toast.error(t('admin.test_logged'));
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const smtp = () => form.mail_driver === 'smtp';

  return (
    <form class="grid gap-6" onSubmit={save}>
      <Card>
        <CardHeader>
          <CardTitle>{t('admin.mail')}</CardTitle>
          <CardDescription>{t('admin.mail_hint')}</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4 sm:grid-cols-2">
          <Field label={t('admin.mail_driver')} error={errors().mail_driver} class="sm:col-span-2">
            <Select value={form.mail_driver} onChange={(e) => setForm('mail_driver', e.currentTarget.value as MailDriver)}>
              <For each={DRIVERS}>{(d) => <option value={d}>{t(`admin.mail_driver_${d}`)}</option>}</For>
            </Select>
          </Field>
          <Show when={smtp()}>
            <Field label={t('admin.smtp_host')} error={errors().smtp_host}>
              <Input required value={form.smtp_host} placeholder="smtp.purelymail.com" onInput={(e) => setForm('smtp_host', e.currentTarget.value)} />
            </Field>
            <div class="grid grid-cols-2 gap-4">
              <Field label={t('admin.smtp_port')} error={errors().smtp_port}>
                <Input type="number" min="1" max="65535" value={form.smtp_port} onInput={(e) => setForm('smtp_port', Number(e.currentTarget.value))} />
              </Field>
              <Field label={t('admin.smtp_security')} error={errors().smtp_security}>
                <Select value={form.smtp_security} onChange={(e) => setForm('smtp_security', e.currentTarget.value as SmtpSecurity)}>
                  <For each={SECURITY}>{(x) => <option value={x}>{t(`admin.smtp_security_${x}`)}</option>}</For>
                </Select>
              </Field>
            </div>
            <Field label={t('admin.smtp_user')} error={errors().smtp_user}>
              <Input autocomplete="off" value={form.smtp_user} onInput={(e) => setForm('smtp_user', e.currentTarget.value)} />
            </Field>
            <Field
              label={
                <span class="flex items-center gap-2">
                  {t('admin.smtp_password')}
                  <Badge variant={passwordSet() ? 'secondary' : 'outline'}>{passwordSet() ? t('admin.password_set') : t('admin.password_not_set')}</Badge>
                </span>
              }
              hint={t('admin.smtp_password_hint')}
              error={errors().smtp_password}
            >
              <Input
                type="password"
                autocomplete="new-password"
                disabled={form.clear_smtp_password}
                value={form.smtp_password ?? ''}
                onInput={(e) => setForm('smtp_password', e.currentTarget.value)}
              />
            </Field>
            <Show when={passwordSet()}>
              <Checkbox
                class="sm:col-span-2"
                label={t('admin.clear_password')}
                checked={form.clear_smtp_password}
                onChange={(e) => setForm('clear_smtp_password', e.currentTarget.checked)}
              />
            </Show>
          </Show>
          <Field label={t('admin.mail_from')} error={errors().mail_from}>
            <Input type="email" required={smtp()} value={form.mail_from} onInput={(e) => setForm('mail_from', e.currentTarget.value)} />
          </Field>
          <Field label={t('admin.mail_from_name')}>
            <Input value={form.mail_from_name} onInput={(e) => setForm('mail_from_name', e.currentTarget.value)} />
          </Field>
        </CardContent>
      </Card>
      <div class="grid gap-2">
        <div class="flex flex-wrap gap-2">
          <Button type="submit" disabled={busy() || !dirty()}>{busy() ? t('common.saving') : t('common.save')}</Button>
          <Button type="button" variant="outline" disabled={busy()} onClick={test}>
            <Send /> {dirty() ? t('admin.save_and_test') : t('admin.send_test')}
          </Button>
        </div>
        <p class="text-xs text-muted-foreground">
          <Show when={saved().mail_driver === 'smtp' || (dirty() && form.mail_driver === 'smtp')} fallback={t('admin.test_hint_log')}>
            {t('admin.test_hint')}
          </Show>
        </p>
      </div>
    </form>
  );
}
