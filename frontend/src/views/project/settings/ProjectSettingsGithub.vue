<script lang="ts" setup>
import {computed, ref, watchEffect} from 'vue'
import {useRoute} from 'vue-router'
import {useI18n} from 'vue-i18n'
import {useTitle} from '@vueuse/core'
import {useQuery} from '@tanstack/vue-query'

import CreateEdit from '@/components/misc/CreateEdit.vue'
import {useBaseStore} from '@/stores/base'
import {projectQuery} from '@/client/queries/projects'
import {
	projectGithubReposQuery,
	useCreateGithubRepoMutation,
	useDeleteGithubRepoMutation,
} from '@/client/queries/github'

// Webpart GitHub integration: which repositories this project's tasks are matched against.
defineOptions({name: 'ProjectSettingsGithub'})
const {t} = useI18n({useScope: 'global'})
useTitle(t('webpart.github.settingsTitle'))

const route = useRoute()
const baseStore = useBaseStore()
const projectId = computed(() => Number(route.params.projectId))
const {data: project} = useQuery(computed(() => ({...projectQuery(projectId.value), enabled: projectId.value > 0})))
watchEffect(() => {
	if (project.value?.id === projectId.value) baseStore.setCurrentProject(project.value)
})

const repos = useQuery(computed(() => projectGithubReposQuery(projectId.value)))
const createMutation = useCreateGithubRepoMutation()
const deleteMutation = useDeleteGithubRepoMutation()

const input = ref('')
const inputError = ref('')

// Accepts "owner/repo" or a GitHub URL.
function parseRepo(value: string): {owner: string, repo: string} | null {
	const cleaned = value.trim()
		.replace(/^https?:\/\/(www\.)?github\.com\//i, '')
		.replace(/\.git$/i, '')
		.replace(/\/+$/, '')
	const parts = cleaned.split('/')
	if (parts.length !== 2 || !parts[0] || !parts[1]) return null
	return {owner: parts[0], repo: parts[1]}
}

async function addRepo() {
	inputError.value = ''
	const parsed = parseRepo(input.value)
	if (!parsed) {
		inputError.value = t('webpart.github.repoFormat')
		return
	}
	try {
		await createMutation.mutateAsync({projectId: projectId.value, ...parsed})
		input.value = ''
	} catch {
		// toasted by the mutation
	}
}
</script>

<template>
	<CreateEdit
		:title="$t('webpart.github.settingsTitle')"
		:has-primary-action="false"
	>
		<p class="github-settings__intro">
			{{ $t('webpart.github.settingsIntro', {key: project?.identifier ? `${project.identifier}-123` : 'HS-123'}) }}
		</p>

		<p
			v-if="project && !project.identifier"
			class="github-settings__warning"
		>
			{{ $t('webpart.github.needsIdentifier') }}
		</p>

		<ul
			v-if="(repos.data.value?.length ?? 0) > 0"
			class="github-settings__list"
		>
			<li
				v-for="repo in repos.data.value"
				:key="repo.id"
				class="github-settings__repo"
			>
				<Icon icon="code-branch" />
				<a
					:href="`https://github.com/${repo.owner}/${repo.repo}`"
					target="_blank"
					rel="noopener"
				>{{ repo.owner }}/{{ repo.repo }}</a>
				<XButton
					variant="tertiary"
					class="has-text-danger github-settings__remove"
					:loading="deleteMutation.isPending.value"
					@click="deleteMutation.mutate({projectId, repoId: repo.id ?? 0})"
				>
					{{ $t('webpart.github.unlink') }}
				</XButton>
			</li>
		</ul>
		<p
			v-else-if="repos.isSuccess.value"
			class="github-settings__empty"
		>
			{{ $t('webpart.github.noReposYet') }}
		</p>

		<form
			class="github-settings__add"
			@submit.prevent="addRepo"
		>
			<label
				for="githubRepoInput"
				class="is-sr-only"
			>{{ $t('webpart.github.repository') }}</label>
			<input
				id="githubRepoInput"
				v-model="input"
				class="input"
				type="text"
				placeholder="Webpart-ro/hunting-store.ro"
				spellcheck="false"
				autocomplete="off"
			>
			<XButton
				type="submit"
				:loading="createMutation.isPending.value"
				:disabled="!input.trim()"
			>
				{{ $t('webpart.github.linkRepo') }}
			</XButton>
		</form>
		<p
			v-if="inputError"
			class="github-settings__error"
		>
			{{ inputError }}
		</p>
	</CreateEdit>
</template>

<style lang="scss" scoped>
.github-settings__intro,
.github-settings__empty {
	color: var(--text-muted);
	font-size: .9rem;
	margin-block-end: 1rem;
}

.github-settings__warning {
	padding: .6rem .8rem;
	border-radius: var(--radius-md);
	background: hsla(38, 92%, 50%, .12);
	color: hsl(32, 81%, 29%);
	font-size: .875rem;
	margin-block-end: 1rem;
}

.github-settings__list {
	list-style: none;
	margin: 0 0 1rem;
	padding: 0;
	border: 1px solid var(--grey-200);
	border-radius: var(--radius-md);
}

.github-settings__repo {
	display: flex;
	align-items: center;
	gap: .6rem;
	padding: .5rem .75rem;

	& + & {
		border-block-start: 1px solid var(--grey-200);
	}

	a {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: .875rem;
	}
}

.github-settings__remove {
	margin-inline-start: auto;
}

.github-settings__add {
	display: flex;
	gap: .5rem;

	.input {
		flex: 1;
	}
}

.github-settings__error {
	margin-block-start: .5rem;
	color: var(--danger-text);
	font-size: .85rem;
}
</style>
