import { init, addMessages, locale, getLocaleFromNavigator } from 'svelte-i18n';
import { browser } from '$app/environment';
import en from './en.json';
import id from './id.json';

addMessages('en', en);
addMessages('id', id);

// Read saved locale from cookie (browser-side)
function getSavedLocale(): string {
  if (!browser) return 'en';
  const match = document.cookie.match(/(?:^|; )simaops_locale=([^;]*)/);
  if (match) return decodeURIComponent(match[1]);
  // Auto-detect from browser
  const nav = getLocaleFromNavigator();
  if (nav?.startsWith('id')) return 'id';
  return 'en';
}

init({
  fallbackLocale: 'en',
  initialLocale: getSavedLocale()
});

// Persist locale changes to cookie
if (browser) {
  locale.subscribe((value) => {
    if (value) {
      document.cookie = `simaops_locale=${value}; path=/; max-age=31536000; samesite=lax`;
    }
  });
}
