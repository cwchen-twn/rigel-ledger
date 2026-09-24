import type { Commodity } from '~/api/types';
import type { SearchOption } from '~/components/ui/search-select';

/** "USD" with "US Dollar" beside it; both are searched. */
export function commodityOptions(list: Commodity[] | undefined): SearchOption[] {
  return (list ?? []).map((c) => ({ value: c.code, label: c.code, description: c.name }));
}

export function timeZoneOptions(zones: string[]): SearchOption[] {
  // "America/Asuncion" is found by "asun" and by "america asuncion".
  return zones.map((z) => ({ value: z, label: z, description: z.includes('_') ? z.replace(/_/g, ' ') : undefined }));
}
