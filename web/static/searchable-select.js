/**
 * Reusable Searchable Select Component
 * Usage: x-data="searchableSelect(options)"
 *
 * @param {Object} config - Configuration object
 * @param {string} config.apiUrl - API endpoint URL
 * @param {string} config.idField - Field name for the ID in API response
 * @param {string} config.textField - Field name for the text in API response
 * @param {string} config.selectedValue - Initial selected value
 * @param {string} config.selectedText - Initial selected text
 * @param {number} config.maxResults - Maximum number of results to show (default: 10)
 */
function searchableSelect(config = {}) {
    const {
        apiUrl = '',
        idField = 'id',
        textField = 'text',
        selectedValue = '',
        selectedText = '',
        maxResults = 10,
    } = config;

    return {
        // State
        options: [],
        selected: selectedText,
        selectedValue: selectedValue,
        isOpen: false,
        initialized: false,
        loading: false,
        error: null,

        // Methods
        async fetchData() {
            if (this.initialized || !apiUrl) return;

            this.loading = true;
            this.error = null;

            try {
                const response = await fetch(apiUrl);
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const data = await response.json();

                this.options = data.map(item => ({
                    id: item[idField],
                    text: item[textField],
                }));

                this.initialized = true;
            } catch (err) {
                console.error('Error fetching data:', err);
                this.error = err.message;
            } finally {
                this.loading = false;
            }
        },

        selectOption(option) {
            this.selected = option.text;
            this.selectedValue = option.id;
            this.isOpen = false;

            // Also dispatch an event for the parent to listen to
            this.$dispatch('searchable-select-changed', {
                option: option,
                value: this.selectedValue,
                text: this.selected
            });
        },

        openDropdown() {
            if (!this.initialized) {
                this.fetchData();
            }
            this.isOpen = true;
        },

        closeDropdown() {
            // Small delay to allow click events to fire
            setTimeout(() => {
                this.isOpen = false;
            }, 150);
        },

        // Computed properties
        get filteredOptions() {
            if (!this.selected) return this.options.slice(0, maxResults);

            const filtered = this.options.filter(option =>
                option.text.toLowerCase().includes(this.selected.toLowerCase())
            );

            return filtered.slice(0, maxResults);
        },

        get hasResults() {
            return this.filteredOptions.length > 0;
        },

        get showNoResults() {
            return this.selected && this.filteredOptions.length === 0 && !this.loading;
        },

        // Validation
        validateField(element) {
            const selected = this.selected;
            const selectedInOptions = this.options.some(option => option.text === selected);
            if (element.checkValidity() && selectedInOptions) {
                element.classList.add('is-valid');
                element.classList.remove('is-invalid');
            } else {
                element.classList.add('is-invalid');
                element.classList.remove('is-valid');
            }
        },
    };
}
