// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
  namespace App {
    interface Locals {
      user?: {
        sub: string;
        username: string;
        email: string;
        name: string;
        roles: string[];
      };
      accessToken?: string;
      // The real signed-in admin while impersonating (locals.user is then the
      // impersonated identity). Undefined when not impersonating.
      realUser?: {
        sub: string;
        username: string;
        email: string;
        name: string;
        roles: string[];
      };
      // Resolved page locale (en | id), set by hooks.server.ts from the
      // simaops_locale cookie. Used by the page-chunk transform to set
      // <html lang="…"> so screen readers pick correct pronunciation.
      lang?: string;
    }
  }
}

export {};
