/// <reference types="@sveltejs/kit" />
declare global {
	namespace App {
		interface Error {
			message: string;
		}
		interface Locals {
			user: import('$lib/types').User | null;
			csrfToken: string;
		}
	}
}

export {};
