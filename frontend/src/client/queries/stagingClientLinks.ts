import {
	queryOptions,
	useMutation,
} from '@tanstack/vue-query'
import {
	stagingClientLinksCreate,
	stagingClientLinksDelete,
	stagingClientLinksList,
	type StagingClientLink,
} from '@/client/generated'
import {contextMutationOptions} from './contextMutation'
import {i18n} from '@/i18n'

// Webpart staging client links: private links that open one staging site for a client.

export type StagingClientLinkInput = {
	label: string
	host: string
	days?: number
}

export const stagingClientLinkKeys = {
	all: ['stagingClientLinks'] as const,
}

export function stagingClientLinksQuery() {
	return queryOptions({
		queryKey: stagingClientLinkKeys.all,
		queryFn: async ({signal}): Promise<StagingClientLink[]> => {
			const {data} = await stagingClientLinksList({signal})
			return data.items ?? []
		},
		staleTime: 0,
	})
}

export function useCreateStagingClientLinkMutation() {
	return useMutation(contextMutationOptions({
		mutationFn: async (body: StagingClientLinkInput) => (await stagingClientLinksCreate({body})).data,
		onSettled: (_body, client) => client.invalidateQueries({queryKey: stagingClientLinkKeys.all}),
	}))
}

export function useDeleteStagingClientLinkMutation() {
	return useMutation(contextMutationOptions({
		mutationFn: async (id: number) => {
			await stagingClientLinksDelete({path: {link: id}})
		},
		onSuccess: (_data, id, client) => client.setQueryData<StagingClientLink[]>(
			stagingClientLinkKeys.all,
			current => current?.filter(link => link.id !== id),
		),
		onSettled: (_id, client) => client.invalidateQueries({queryKey: stagingClientLinkKeys.all}),
		successMessage: () => i18n.global.t('webpart.stagingLinks.revokeSuccess'),
	}))
}
