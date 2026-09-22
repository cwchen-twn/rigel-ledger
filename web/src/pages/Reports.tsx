import type { JSXElement } from 'solid-js';
import { useI18n } from '../i18n';

export default function Reports(): JSXElement {
  const { t } = useI18n();
  return (
    <div class="container-fluid py-4">
      <h4 class="mb-4">{t('reports.title')}</h4>
      <div class="card">
        <div class="card-body text-center text-muted py-5">
          <i class="bi bi-bar-chart fs-1 mb-3 d-block"></i>
          <p>{t('reports.coming_soon')}</p>
        </div>
      </div>
    </div>
  );
}
