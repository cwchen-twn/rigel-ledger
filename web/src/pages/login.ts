import Alpine from 'alpinejs';

Alpine.data('loginForm', () => ({
  username: '' as string,
  password: '' as string,
  isSubmitting: false as boolean,

  validateField(element: HTMLInputElement): void {
    if (!element.checkValidity()) {
      element.classList.add('is-invalid');
      element.classList.remove('is-valid');
    } else {
      element.classList.add('is-valid');
      element.classList.remove('is-invalid');
    }
  },

  async handleSubmit(): Promise<void> {
    const form = document.getElementById('login-form') as HTMLFormElement;
    if (!form.checkValidity()) {
      form.querySelectorAll<HTMLInputElement>('#login-form input').forEach(el => this.validateField(el));
      return;
    }
    form.classList.add('was-validated');
    this.isSubmitting = true;
    try {
      const formData = new URLSearchParams(new FormData(form) as unknown as Record<string, string>);
      const response = await fetch('/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: formData,
      });
      if (response.ok) {
        window.location.href = `/${this.username}`;
      } else {
        const errorEl = document.getElementById('error-message');
        if (errorEl) {
          errorEl.classList.remove('d-none');
          errorEl.textContent = await response.text();
        }
      }
    } catch {
      const errorEl = document.getElementById('error-message');
      if (errorEl) {
        errorEl.classList.remove('d-none');
        errorEl.textContent = 'An error occurred during login. Please try again.';
      }
    } finally {
      this.isSubmitting = false;
    }
  },
}));
