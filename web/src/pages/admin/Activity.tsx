import { createResource } from 'solid-js';
import { api } from '~/api/client';
import { Card, CardContent, CardHeader, CardTitle } from '~/components/ui/card';
import { EventList } from '~/components/EventList';
import { useI18n } from '~/i18n';

export function Activity() {
  const { t } = useI18n();
  const [events] = createResource(() => api.admin.events(200));
  return (
    <Card>
      <CardHeader><CardTitle>{t('admin.tab_activity')}</CardTitle></CardHeader>
      <CardContent>
        <EventList events={events() ?? []} showUser />
      </CardContent>
    </Card>
  );
}
