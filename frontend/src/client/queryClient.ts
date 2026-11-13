import {CancelledError, QueryCache, QueryClient, type QueryMeta} from '@tanstack/vue-query'

import {isRequestContextAbort} from '@/client/requestContext'
import {isInTrashError} from '@/client/queries/trashErrors'
import {error} from '@/message'
import {TOTP_ENROLLMENT_REQUIRED} from '@/constants/webpartErrorCodes'

declare module '@tanstack/vue-query' {
	interface Register {
		queryMeta: {
			handlesError?: boolean | ((cause: unknown) => boolean),
		}
	}
}

function handlesError(meta: QueryMeta | undefined, cause: unknown): boolean {
	const handles = meta?.handlesError
	return typeof handles === 'function' ? handles(cause) : handles === true
}

export const queryClient = new QueryClient({
	queryCache: new QueryCache({
		onError: (cause, query) => {
			if (handlesError(query.meta, cause)) return
			if (cause instanceof CancelledError || isRequestContextAbort(cause)) return
			error(cause)
		},
	}),
	defaultOptions: {
		queries: {
			staleTime: 60_000,
			retry: (failureCount, cause) => !isInTrashError(cause)
				&& (cause as {code?: unknown} | null)?.code !== TOTP_ENROLLMENT_REQUIRED
				&& failureCount < 1,
			refetchOnWindowFocus: false,
		},
		mutations: {
			retry: false,
		},
	},
})
