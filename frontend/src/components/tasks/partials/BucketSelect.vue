<template>
	<!-- Webpart: prominent status pill (Jira-style) for the task sidebar -->
	<div
		v-if="variant === 'status'"
		class="status-select"
	>
		<Dropdown v-if="kanbanView && canWrite">
			<template #trigger="{toggleOpen, open}">
				<BaseButton
					class="status-pill"
					:class="statusClass"
					:aria-expanded="open"
					:aria-label="$t('task.detail.bucketSelectLabel', {bucket: statusTitle})"
					@click="toggleOpen"
				>
					<span>{{ statusTitle }}</span>
					<Icon
						icon="chevron-down"
						class="status-pill__chevron"
					/>
				</BaseButton>
			</template>
			<template #default="{close}">
				<div
					class="status-menu"
					@click="close"
				>
					<DropdownItem
						v-for="bucket in buckets"
						:key="bucket.id"
						:class="{'is-active': currentBucket?.id === bucket.id}"
						@click="changeBucket(bucket)"
					>
						{{ bucket.title }}
					</DropdownItem>
				</div>
			</template>
		</Dropdown>
		<span
			v-else
			class="status-pill is-static"
			:class="statusClass"
		>{{ statusTitle }}</span>
	</div>
	<template v-else-if="kanbanView">
		<span
			class="breadcrumb-separator"
			aria-hidden="true"
		>/</span>
		<template v-if="canWrite">
			<Dropdown>
				<template #trigger="{toggleOpen, open}">
					<BaseButton
						class="bucket-name"
						:aria-expanded="open"
						:aria-label="$t('task.detail.bucketSelectLabel', {bucket: currentBucketTitle})"
						@click="toggleOpen"
					>
						{{ currentBucketTitle }}
						<Icon
							icon="pencil-alt"
							class="change-indicator d-print-none"
						/>
					</BaseButton>
				</template>
				<template #default="{close}">
					<div @click="close">
						<DropdownItem
							v-for="bucket in buckets"
							:key="bucket.id"
							:class="{'is-active': currentBucket?.id === bucket.id}"
							@click="changeBucket(bucket)"
						>
							{{ bucket.title }}
						</DropdownItem>
					</div>
				</template>
			</Dropdown>
		</template>
		<span
			v-else
			class="bucket-name"
		>
			{{ currentBucketTitle }}
		</span>
	</template>
</template>

<script lang="ts" setup>
import {bucketsQuery} from '@/client/queries/kanban'
import {useMoveTaskMutation} from '@/client/queries/taskMutations'
import {useQuery} from '@tanstack/vue-query'
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import type {Task as ITask} from '@/client/generated'
import type {Bucket as IBucket} from '@/client/generated'

import {PROJECT_VIEW_KINDS} from '@/constants/projectView'

import {useProjects} from '@/composables/useProjects'
import {useBaseStore} from '@/stores/base'

import BaseButton from '@/components/base/BaseButton.vue'
import Dropdown from '@/components/misc/Dropdown.vue'
import DropdownItem from '@/components/misc/DropdownItem.vue'


import {success} from '@/message'

const props = withDefaults(defineProps<{
	task: ITask
	canWrite: boolean
	variant?: 'breadcrumb' | 'status'
}>(), {
	variant: 'breadcrumb',
})


const {t} = useI18n({useScope: 'global'})

const projectList = useProjects()
const moveMutation = useMoveTaskMutation()
const baseStore = useBaseStore()

const project = computed(() => projectList.projects[props.task.project_id ?? 0])

// If the project has exactly one manual kanban view, always use it.
// If there are multiple, only show the selector when the active view is one of them.
const kanbanView = computed(() => {
	if (!project.value?.views) {
		return null
	}

	const manualKanbanViews = project.value.views.filter(
		view => view.view_kind === PROJECT_VIEW_KINDS.KANBAN
			&& view.bucket_configuration_mode === 'manual',
	)

	if (manualKanbanViews.length === 1) {
		return manualKanbanViews[0]
	}

	if (manualKanbanViews.length > 1) {
		const activeViewId = baseStore.currentProjectViewId
		return manualKanbanViews.find(v => v.id === activeViewId) || null
	}

	return null
})

const bucketsQueryResult = useQuery(computed(() => bucketsQuery(props.task.project_id ?? 0, kanbanView.value?.id ?? 0)))
const buckets = computed(() => bucketsQueryResult.data.value ?? [])

const currentBucket = computed(() => {
	if (!kanbanView.value) {
		return undefined
	}
	return props.task.buckets?.find(b => b.project_view_id === kanbanView.value?.id)
})

const currentBucketTitle = computed(() => {
	return currentBucket.value?.title || t('task.detail.noBucket')
})

// Status pill: the kanban column, or plain Open/Done when the project has no manual board.
const statusTitle = computed(() => {
	if (kanbanView.value && currentBucket.value?.title) return currentBucket.value.title
	return props.task.done ? t('task.attributes.done') : t('webpart.list.open')
})

const statusClass = computed(() => {
	const doneBucketId = kanbanView.value?.done_bucket_id
	if (props.task.done || (doneBucketId && currentBucket.value?.id === doneBucketId)) return 'is-done'
	if (!currentBucket.value || currentBucket.value.id === buckets.value[0]?.id) return 'is-todo'
	return 'is-progress'
})

async function changeBucket(bucket: IBucket) {
	if (!kanbanView.value || currentBucket.value?.id === bucket.id) {
		return
	}

	await moveMutation.mutateAsync({
		project: props.task.project_id!,
		view: kanbanView.value.id!,
		bucket: bucket.id!,
		task: props.task,
	})

	success({message: t('task.detail.bucketChangedSuccess')})
}
</script>

<style lang="scss" scoped>
.breadcrumb-separator {
	color: var(--grey-300);
	margin-inline: .1rem;
}

.bucket-name {
	color: var(--grey-800);

	&:hover {
		color: var(--primary);
	}
}

.change-indicator {
	font-size: .75em;
	margin-inline-start: .25rem;
	color: var(--grey-400);
}

:deep(.dropdown) {
	display: inline;
}

:deep(.dropdown-trigger) {
	display: inline;
	padding: 0;
}

.status-select {
	display: flex;

	:deep(.dropdown) {
		display: inline-flex;
	}
}

.status-pill {
	display: inline-flex;
	align-items: center;
	gap: .5rem;
	min-block-size: 2.25rem;
	padding: .35rem .85rem;
	border-radius: var(--radius-md);
	border: 1px solid transparent;
	font-size: .9rem;
	font-weight: 600;
	transition: filter $transition;

	&:not(.is-static):hover,
	&:not(.is-static):focus-visible {
		filter: brightness(.96);
	}

	&.is-todo {
		background: var(--grey-100);
		border-color: var(--grey-300);
		color: var(--grey-800);
	}

	&.is-progress {
		background: hsla(var(--primary-h), var(--primary-s), 55%, .14);
		border-color: hsla(var(--primary-h), var(--primary-s), 50%, .35);
		color: hsl(var(--primary-h), 70%, 38%);
	}

	&.is-done {
		background: hsla(var(--success-h), 60%, 40%, .14);
		border-color: hsla(var(--success-h), 60%, 35%, .35);
		color: var(--success-text);
	}
}

.status-pill__chevron {
	inline-size: .7rem;
}

.status-menu {
	position: relative; // above Dropdown's hover-bridge ::before
}
</style>
