import { browser } from '$app/environment';
import { init, register, getLocaleFromNavigator } from 'svelte-i18n';

register('en', () => import('./en.json'));
register('id', () => import('./id.json'));

init({
  fallbackLocale: 'en',
  initialLocale: browser ? getLocaleFromNavigator()?.startsWith('id') ? 'id' : 'en' : 'en'
});
