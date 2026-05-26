import { init, addMessages } from 'svelte-i18n';
import en from './en.json';
import id from './id.json';

addMessages('en', en);
addMessages('id', id);

init({
  fallbackLocale: 'en',
  initialLocale: 'en'
});
