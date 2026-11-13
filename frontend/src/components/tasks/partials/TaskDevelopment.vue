<template>
	<!-- Webpart GitHub integration: branches, pull requests and CI that mention this task -->
	<section
		v-if="dev.isSuccess.value && dev.data.value?.configured"
		class="task-development"
		:aria-label="$t('webpart.github.development')"
	>
		<header class="task-development__header">
			<h2 class="task-development__title">
				{{ $t('webpart.github.development') }}
			</h2>
			<span
				v-if="data?.key"
				class="task-development__key"
			>{{ data.key }}</span>
		</header>

		<p
			v-if="!data?.key"
			class="task-development__empty"
		>
			{{ $t('webpart.github.noIdentifier') }}
		</p>

		<p
			v-else-if="repositories.length === 0"
			class="task-development__empty"
		>
			{{ $t('webpart.github.noRepos') }}
			<RouterLink
				v-if="canWrite"
				:to="{name: 'project.settings.github', params: {projectId}}"
			>
				{{ $t('webpart.github.linkRepo') }}
			</RouterLink>
		</p>

		<template v-else>
			<div
				v-for="repo in repositories"
				:key="repo.full_name"
				class="dev-repo"
			>
				<a
					v-if="repositories.length > 1"
					:href="repo.url"
					target="_blank"
					rel="noopener"
					class="dev-repo__name"
				>{{ repo.full_name }}</a>

				<p
					v-if="!repo.accessible"
					class="task-development__empty"
				>
					{{ $t('webpart.github.appNotInstalled', {repo: repo.full_name}) }}
				</p>

				<ul
					v-if="(repo.pull_requests?.length ?? 0) > 0"
					class="dev-list"
				>
					<li
						v-for="pr in repo.pull_requests"
						:key="`pr-${pr.number}`"
					>
						<a
							:href="pr.url"
							target="_blank"
							rel="noopener"
							class="dev-item"
						>
							<Icon
								:icon="pr.state === 'merged' ? 'code-merge' : 'code-pull-request'"
								class="dev-item__icon"
								:class="`is-${pr.state}`"
							/>
							<span class="dev-item__text">
								<span class="dev-item__title">#{{ pr.number }} {{ pr.title }}</span>
								<span class="dev-item__meta">
									<span
										class="dev-badge"
										:class="`is-${pr.draft && pr.state === 'open' ? 'draft' : pr.state}`"
									>{{ prStateLabel(pr) }}</span>
									<span>{{ pr.head_ref }} → {{ pr.base_ref }}</span>
								</span>
							</span>
							<CiDot
								v-if="pr.ci?.state"
								:ci="pr.ci"
							/>
						</a>
					</li>
				</ul>

				<ul
					v-if="(repo.branches?.length ?? 0) > 0"
					class="dev-list"
				>
					<li
						v-for="branch in repo.branches"
						:key="`branch-${branch.name}`"
					>
						<a
							:href="branch.url"
							target="_blank"
							rel="noopener"
							class="dev-item"
						>
							<Icon
								icon="code-branch"
								class="dev-item__icon"
							/>
							<span class="dev-item__text">
								<span class="dev-item__title dev-item__title--mono">{{ branch.name }}</span>
							</span>
							<CiDot
								v-if="branch.ci?.state"
								:ci="branch.ci"
							/>
						</a>
					</li>
				</ul>

				<p
					v-if="repo.accessible && !(repo.branches?.length) && !(repo.pull_requests?.length)"
					class="task-development__empty"
				>
					{{ $t('webpart.github.nothingYet') }}
				</p>
			</div>

			<!-- Create branch -->
			<template v-if="canWrite">
				<XButton
					v-if="!showCreate"
					variant="secondary"
					icon="code-branch"
					class="task-development__create"
					@click="openCreate"
				>
					{{ $t('webpart.github.createBranch') }}
				</XButton>

				<form
					v-else
					class="create-branch"
					@submit.prevent="createBranch"
				>
					<label
						v-if="accessibleRepos.length > 1"
						class="create-branch__field"
					>
						<span>{{ $t('webpart.github.repository') }}</span>
						<select v-model="form.repo">
							<option
								v-for="repo in accessibleRepos"
								:key="repo.full_name"
								:value="repo.full_name"
							>
								{{ repo.full_name }}
							</option>
						</select>
					</label>
					<label class="create-branch__field">
						<span>{{ $t('webpart.github.baseBranch') }}</span>
						<select
							v-model="form.base"
							:disabled="bases.isPending.value"
						>
							<option
								v-for="name in bases.data.value?.branches ?? []"
								:key="name"
								:value="name"
							>
								{{ name }}
							</option>
						</select>
					</label>
					<label class="create-branch__field">
						<span>{{ $t('webpart.github.branchName') }}</span>
						<input
							v-model="form.name"
							type="text"
							class="input"
							spellcheck="false"
							autocomplete="off"
						>
					</label>
					<div class="create-branch__actions">
						<XButton
							type="submit"
							:loading="createMutation.isPending.value"
							:disabled="!form.repo || !form.name || !form.base"
						>
							{{ $t('webpart.github.create') }}
						</XButton>
						<XButton
							variant="tertiary"
							@click="showCreate = false"
						>
							{{ $t('misc.cancel') }}
						</XButton>
					</div>
				</form>
			</template>
		</template>
	</section>
</template>

<script setup lang="ts">
import {computed, reactive, ref, watch} from 'vue'
import {useQuery} from '@tanstack/vue-query'
import {useI18n} from 'vue-i18n'

import CiDot from '@/components/tasks/partials/CiDot.vue'
import {branchBasesQuery, taskDevelopmentQuery, useCreateBranchMutation} from '@/client/queries/github'
import type {PullRequest} from '@/client/generated'

const props = defineProps<{
	taskId: number
	taskTitle: string
	projectId: number
	canWrite: boolean
}>()

const {t} = useI18n()

const dev = useQuery(computed(() => taskDevelopmentQuery(props.taskId)))
const data = computed(() => dev.data.value)
const repositories = computed(() => data.value?.repositories ?? [])
const accessibleRepos = computed(() => repositories.value.filter(r => r.accessible))

function prStateLabel(pr: PullRequest) {
	if (pr.state === 'open' && pr.draft) return t('webpart.github.draft')
	return t(`webpart.github.state.${pr.state}`)
}

// Branch name from the task: "HS-334-release-day-customer-service".
function slugify(text: string) {
	return text
		.normalize('NFD').replace(/[̀-ͯ]/g, '') // strip diacritics (ă, ș, ț…)
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 50)
		.replace(/-+$/g, '')
}

const showCreate = ref(false)
const form = reactive({repo: '', base: '', name: ''})

const bases = useQuery(computed(() => branchBasesQuery(props.taskId, showCreate.value ? form.repo : '')))
watch(() => bases.data.value, value => {
	if (value && !form.base) form.base = value.default_base ?? ''
})

function openCreate() {
	form.repo = accessibleRepos.value[0]?.full_name ?? ''
	form.base = ''
	const slug = slugify(props.taskTitle)
	form.name = slug ? `${data.value?.key}-${slug}` : (data.value?.key ?? '')
	showCreate.value = true
}

watch(() => form.repo, () => {
	form.base = ''
})

const createMutation = useCreateBranchMutation()

async function createBranch() {
	try {
		await createMutation.mutateAsync({taskId: props.taskId, repo: form.repo, base: form.base, name: form.name.trim()})
		showCreate.value = false
	} catch {
		// toasted by the mutation
	}
}
</script>

<style lang="scss" scoped>
.task-development {
	margin-block-end: .75rem;
	padding: .75rem;
	border: 1px solid var(--grey-200);
	border-radius: var(--radius-md);
	background: var(--white);
}

.task-development__header {
	display: flex;
	align-items: center;
	gap: .5rem;
	margin-block-end: .5rem;
}

.task-development__title {
	margin: 0;
	font-size: .85rem;
	font-weight: 600;
	color: var(--text-strong);
}

.task-development__key {
	margin-inline-start: auto;
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	font-size: .7rem;
	color: var(--text-muted);
}

.task-development__empty {
	margin: .25rem 0;
	font-size: .8rem;
	color: var(--text-muted);
}

.task-development__create {
	inline-size: 100%;
	margin-block-start: .5rem;
}

.dev-repo + .dev-repo {
	margin-block-start: .5rem;
	padding-block-start: .5rem;
	border-block-start: 1px solid var(--grey-200);
}

.dev-repo__name {
	display: block;
	font-size: .75rem;
	font-weight: 600;
	color: var(--text-muted);
	margin-block-end: .25rem;
}

.dev-list {
	list-style: none;
	margin: 0;
	padding: 0;
}

.dev-item {
	display: flex;
	align-items: flex-start;
	gap: .5rem;
	padding: .4rem .35rem;
	margin-inline: -.35rem;
	border-radius: var(--radius-sm);
	color: var(--text);

	&:hover,
	&:focus-visible {
		background: var(--grey-50);
		color: var(--text-strong);
	}
}

.dev-item__icon {
	flex-shrink: 0;
	inline-size: .8rem;
	margin-block-start: .2rem;
	color: var(--grey-500);

	&.is-open {
		color: hsl(142, 64%, 30%);
	}

	&.is-merged {
		color: hsl(262, 52%, 47%);
	}

	&.is-closed {
		color: var(--danger-text);
	}
}

.dev-item__text {
	flex: 1;
	min-inline-size: 0;
	display: flex;
	flex-direction: column;
	gap: .15rem;
}

.dev-item__title {
	font-size: .8rem;
	line-height: 1.35;
	overflow: hidden;
	text-overflow: ellipsis;
	display: -webkit-box;
	-webkit-line-clamp: 2;
	-webkit-box-orient: vertical;

	&--mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: .75rem;
		white-space: nowrap;
		display: block;
	}
}

.dev-item__meta {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: .35rem;
	font-size: .7rem;
	color: var(--text-muted);
}

.dev-badge {
	padding: .05rem .4rem;
	border-radius: 999px;
	font-weight: 600;
	font-size: .65rem;
	text-transform: uppercase;
	letter-spacing: .03em;

	&.is-open {
		background: hsla(142, 64%, 40%, .14);
		color: hsl(142, 64%, 26%);
	}

	&.is-merged {
		background: hsla(262, 52%, 50%, .14);
		color: hsl(262, 52%, 40%);
	}

	&.is-closed {
		background: hsla(var(--danger-h), var(--danger-s), 50%, .12);
		color: var(--danger-text);
	}

	&.is-draft {
		background: var(--grey-100);
		color: var(--text-muted);
	}
}

.create-branch {
	display: flex;
	flex-direction: column;
	gap: .5rem;
	margin-block-start: .5rem;
	padding-block-start: .5rem;
	border-block-start: 1px solid var(--grey-200);
}

.create-branch__field {
	display: flex;
	flex-direction: column;
	gap: .2rem;
	font-size: .75rem;
	color: var(--text-muted);

	select,
	input {
		font: inherit;
		font-size: .8rem;
		color: var(--text-strong);
		padding: .35rem .5rem;
		border: 1px solid var(--grey-300);
		border-radius: var(--radius-sm);
		background: var(--white);
	}

	input {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
}

.create-branch__actions {
	display: flex;
	gap: .5rem;
}
</style>
