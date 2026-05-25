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
    }
  }
}

export {};
