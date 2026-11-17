<script setup lang="ts">
import {computed, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'

import {useTitle} from '@/composables/useTitle'
import {useCopyToClipboard} from '@/composables/useCopyToClipboard'
import {formatDateSince, formatDisplayDate} from '@/helpers/time/formatDate'
import {
	stagingClientLinksQuery,
	useCreateStagingClientLinkMutation,
	useDeleteStagingClientLinkMutation,
} from '@/client/queries/stagingClientLinks'
import type {StagingClientLink} from '@/client/generated'
import ErrorMessage from '@/components/misc/Error.vue'
import Message from '@/components/misc/Message.vue'
import FormField from '@/components/input/FormField.vue'
import XButton from '@/components/input/Button.vue'

// Webpart: private links that open one staging site for a client without a Tasks account.
defineOptions({name: 'StagingClientLinks'})

const {t} = useI18n({useScope: 'global'})
useTitle(() => `${t('webpart.stagingLinks.title')} - ${t('user.settings.title')}`)

const linksQuery = useQuery(computed(() => stagingClientLinksQuery()))
const links = computed(() => linksQuery.data.value ?? [])
const createMutation = useCreateStagingClientLinkMutation()
const deleteMutation = useDeleteStagingClientLinkMutation()
const copy = useCopyToClipboard()

const label = ref('')
const host = ref('')
const days = ref(30)
const createdUrl = ref('')

const canCreate = computed(() => label.value.trim() !== '' && host.value.trim() !== '' && !createMutation.isPending.value)

async function createLink() {
	if (!canCreate.value) return
	createdUrl.value = ''
	try {
		const created = await createMutation.mutateAsync({
			label: label.value.trim(),
			host: host.value.trim(),
			days: days.value,
		})
		createdUrl.value = created.url ?? ''
		label.value = ''
		host.value = ''
	} catch { return }
}

function isExpired(link: StagingClientLink) {
	return link.expires_at !== undefined && new Date(link.expires_at).getTime() <= Date.now()
}

const linkToRevoke = ref<StagingClientLink | null>(null)

async function revokeLink() {
	const id = linkToRevoke.value?.id
	if (id === undefined || deleteMutation.isPending.value) return
	try {
		await deleteMutation.mutateAsync(id)
	} catch { return }
	linkToRevoke.value = null
}
</script>

<template>
	<Card
		:title="$t('webpart.stagingLinks.title')"
		:loading="linksQuery.isPending.value"
	>
		<p class="mbe-4">
			{{ $t('webpart.stagingLinks.description') }}
		</p>

		<form
			class="mbe-4"
			@submit.prevent="createLink"
		>
			<FormField
				id="stagingLinkLabel"
				v-model="label"
				:label="$t('webpart.stagingLinks.label')"
				:placeholder="$t('webpart.stagingLinks.labelPlaceholder')"
				type="text"
			/>
			<FormField
				id="stagingLinkHost"
				v-model="host"
				:label="$t('webpart.stagingLinks.host')"
				placeholder="huntingstore-ro-staging.webpart.ro"
				type="text"
			/>
			<div class="field">
				<label
					class="label"
					for="stagingLinkDays"
				>
					{{ $t('webpart.stagingLinks.validFor') }}
				</label>
				<div class="control select">
					<select
						id="stagingLinkDays"
						v-model.number="days"
					>
						<option :value="7">
							{{ $t('webpart.stagingLinks.days', {count: 7}) }}
						</option>
						<option :value="30">
							{{ $t('webpart.stagingLinks.days', {count: 30}) }}
						</option>
						<option :value="90">
							{{ $t('webpart.stagingLinks.days', {count: 90}) }}
						</option>
						<option :value="365">
							{{ $t('webpart.stagingLinks.days', {count: 365}) }}
						</option>
					</select>
				</div>
			</div>
			<XButton
				:disabled="!canCreate"
				:loading="createMutation.isPending.value"
				@click="createLink"
			>
				{{ $t('webpart.stagingLinks.create') }}
			</XButton>
		</form>

		<Message
			v-if="createdUrl"
			variant="success"
			class="mbe-4"
		>
			<p class="mbe-2">
				{{ $t('webpart.stagingLinks.created') }}
			</p>
			<p class="staging-link-url mbe-2">
				<code>{{ createdUrl }}</code>
			</p>
			<XButton
				variant="secondary"
				icon="paste"
				@click="copy(createdUrl)"
			>
				{{ $t('misc.copy') }}
			</XButton>
		</Message>

		<ErrorMessage v-if="linksQuery.isError.value" />

		<div
			v-if="links.length > 0"
			class="has-horizontal-overflow"
		>
			<table class="table">
				<thead>
					<tr>
						<th>{{ $t('webpart.stagingLinks.label') }}</th>
						<th>{{ $t('webpart.stagingLinks.host') }}</th>
						<th>{{ $t('webpart.stagingLinks.expires') }}</th>
						<th>{{ $t('webpart.stagingLinks.lastUsed') }}</th>
						<th class="has-text-end">
							{{ $t('misc.actions') }}
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="link in links"
						:key="link.id"
					>
						<td>{{ link.label }}</td>
						<td>{{ link.host }}</td>
						<td>
							<span
								v-if="isExpired(link)"
								class="tag is-danger"
							>
								{{ $t('webpart.stagingLinks.expired') }}
							</span>
							<template v-else>
								{{ formatDisplayDate(link.expires_at) }}
							</template>
						</td>
						<td>
							{{ link.last_used_at ? formatDateSince(link.last_used_at) : $t('webpart.stagingLinks.never') }}
						</td>
						<td class="has-text-end">
							<XButton
								variant="secondary"
								@click="linkToRevoke = link"
							>
								{{ $t('webpart.stagingLinks.revoke') }}
							</XButton>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<p v-else-if="!linksQuery.isPending.value && !linksQuery.isError.value">
			{{ $t('webpart.stagingLinks.none') }}
		</p>

		<Modal
			:enabled="linkToRevoke !== null"
			@close="linkToRevoke = null"
			@submit="revokeLink()"
		>
			<template #header>
				{{ $t('webpart.stagingLinks.revokeHeader') }}
			</template>
			<template #text>
				<p>
					{{ $t('webpart.stagingLinks.revokeText', {label: linkToRevoke?.label ?? '', host: linkToRevoke?.host ?? ''}) }}
				</p>
			</template>
		</Modal>
	</Card>
</template>

<style lang="scss" scoped>
.staging-link-url code {
	word-break: break-all;
	white-space: normal;
}
</style>
