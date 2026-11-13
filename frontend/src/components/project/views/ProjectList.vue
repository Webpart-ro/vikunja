<template>
	<ProjectWrapper
		class="project-list"
		:is-loading-project="isLoadingProject"
		:project-id="projectId"
		:view-id
	>
		<template #header>
			<div class="filter-container">
				<label class="group-by">
					<span class="group-by__label">{{ $t('webpart.list.groupBy') }}</span>
					<select
						v-model="groupBy"
						class="group-by__select"
					>
						<option
							v-for="option in groupByOptions"
							:key="option.value"
							:value="option.value"
						>
							{{ option.label }}
						</option>
					</select>
				</label>
				<SortPopup
					v-model="sortByParam"
				/>
				<FilterPopup
					v-if="!isSavedFilterProject(project)"
					v-model="params"
					:view-id="viewId"
					:project-id="projectId"
					@update:modelValue="loadTasks()"
				/>
			</div>
		</template>

		<template #default>
			<div
				:class="{ 'is-loading': showLoading }"
				class="loader-container is-max-width-desktop list-view"
			>
				<Card
					:padding="false"
					:has-content="false"
					class="has-overflow"
				>
					<AddTask
						v-if="!project?.is_archived && canWrite"
						ref="addTaskRef"
						class="list-view__add-task d-print-none"
						@tasksAdded="updateTaskList"
					/>

					<Nothing v-if="ctaVisible && tasks.length === 0 && !loading">
						{{ $t('project.list.empty') }}
						<ButtonLink
							v-if="(project?.id ?? 0) > 0 && canWrite"
							@click="focusNewTaskInput()"
						>
							{{ $t('project.list.newTaskCta') }}
						</ButtonLink>
					</Nothing>

					<div
						v-if="tasks && tasks.length > 0"
						class="list-columns-header"
						aria-hidden="true"
					>
						<span class="list-columns-header__task">{{ $t('webpart.list.task') }}</span>
						<span class="list-columns-header__assignees">{{ $t('task.attributes.assignees') }}</span>
						<span class="list-columns-header__due">{{ $t('task.attributes.dueDate') }}</span>
						<span class="list-columns-header__icons" />
					</div>

					<template v-if="groupBy !== 'none' && tasks.length > 0">
						<section
							v-for="(group, groupIndex) in groupedTasks"
							:key="group.key"
							class="task-group"
						>
							<h3 class="task-group__header">
								<span
									v-if="group.color"
									class="task-group__dot"
									:style="{background: group.color}"
									aria-hidden="true"
								/>
								<span class="task-group__title">{{ group.label }}</span>
								<span class="task-group__count">{{ group.tasks.length }}</span>
							</h3>
							<ul class="tasks dragging-disabled">
								<SingleTaskInProject
									v-for="(t, rowIndex) in group.tasks"
									:key="`${group.key}-${t.id}`"
									:ref="(el) => setTaskRef(el as InstanceType<typeof SingleTaskInProject> | null, groupedRowIndex(groupIndex, rowIndex))"
									:show-list-color="false"
									:can-mark-as-done="canWrite || isPseudoProject"
									:the-task="t"
									:all-tasks="allTasks"
									columns
									@taskUpdated="updateTasks"
								/>
							</ul>
						</section>
					</template>

					<draggable
						v-else-if="tasks && tasks.length > 0"
						v-model="tasks"
						:group="{name: 'tasks', put: false}"
						:disabled="!canDragTasks || !isPositionSorting"
						item-key="id"
						tag="ul"
						:component-data="{
							class: {
								tasks: true,
								'dragging-disabled': !canDragTasks || !isPositionSorting
							},
							type: 'transition-group'
						}"
						:animation="100"
						:handle="dragHandle"
						:delay-on-touch-only="!isTouchDevice"
						:delay="isTouchDevice ? 0 : 1000"
						ghost-class="task-ghost"
						@start="handleDragStart"
						@end="saveTaskPosition"
					>
						<template #item="{element: t, index}">
							<SingleTaskInProject
								:ref="(el) => setTaskRef(el as InstanceType<typeof SingleTaskInProject> | null, index)"
								:show-list-color="false"
								:can-mark-as-done="canWrite || isPseudoProject"
								:the-task="t"
								:all-tasks="allTasks"
								columns
								@taskUpdated="updateTasks"
							>
								<span
									v-if="canDragTasks && isPositionSorting"
									class="icon handle"
								>
									<Icon icon="grip-lines" />
								</span>
							</SingleTaskInProject>
						</template>
					</draggable>

					<Pagination
						:total-pages="totalPages"
						:current-page="currentPage"
					/>
				</Card>
			</div>
		</template>
	</ProjectWrapper>
</template>


<script setup lang="ts">
import {useUpdateTaskPositionMutation} from '@/client/queries/taskMutations'
import {ref, computed, nextTick, onMounted, onBeforeUnmount, watch, toRef} from 'vue'
import {draggableFor} from '@/components/base/Draggable'

import ProjectWrapper from '@/components/project/ProjectWrapper.vue'
import ButtonLink from '@/components/misc/ButtonLink.vue'
import AddTask from '@/components/tasks/AddTask.vue'
import SingleTaskInProject from '@/components/tasks/partials/SingleTaskInProject.vue'
import FilterPopup from '@/components/project/partials/FilterPopup.vue'
import Nothing from '@/components/misc/Nothing.vue'
import Pagination from '@/components/misc/Pagination.vue'
import SortPopup from '@/components/project/partials/SortPopup.vue'

import {useTaskList} from '@/composables/useTaskList'
import {useTaskDragToProject} from '@/composables/useTaskDragToProject'
import {useCurrentProject} from '@/composables/useCurrentProject'
import {shouldShowTaskInListView} from '@/composables/useTaskListFiltering'
import {PERMISSIONS as Permissions} from '@/constants/permissions'
import {eventToShortcutString} from '@/helpers/shortcut'
import {calculateItemPosition} from '@/helpers/calculateItemPosition'
import type {TaskResponse} from '@/client/queries/tasks'
import {isSavedFilterProject} from '@/client/queries/projects'

import {useBaseStore} from '@/stores/base'
import {useTaskDragState} from '@/composables/useTaskDragState'
import {useDelayedLoading} from '@/composables/useDelayedLoading'
import {useStorage} from '@vueuse/core'
import {useI18n} from 'vue-i18n'


const props = defineProps<{
	isLoadingProject: boolean,
	projectId: number,
	viewId: number,
}>()

const draggable = draggableFor<TaskResponse>()

const projectId = toRef(props, 'projectId')

defineOptions({name: 'List'})

const ctaVisible = ref(false)

const drag = ref(false)

const {
	tasks: allTasks,
	loading,
	totalPages,
	currentPage,
	loadTasks,
	params,
	sortByParam,
} = useTaskList(
	() => projectId.value,
	() => props.viewId,
	{position: 'asc'},
	() => projectId.value === -1
		? ['comment_count', 'is_unread']
		: ['subtasks', 'comment_count', 'is_unread'],
)
const showLoading = useDelayedLoading(loading)

const positionMutation = useUpdateTaskPositionMutation()

const dragTasks = ref<TaskResponse[] | null>(null)
const tasks = computed({
	get: () => dragTasks.value ?? allTasks.value.filter(task => shouldShowTaskInListView(task, allTasks.value)),
	set: value => { dragTasks.value = value },
})
watch([projectId, () => props.viewId], () => { dragTasks.value = null })

const isPositionSorting = computed(() => 'position' in sortByParam.value)

// Webpart: group the list by assignee / due date / priority / label / status.
// Grouping is a per-view, per-browser preference; drag-to-reorder only works ungrouped.
type GroupBy = 'none' | 'assignee' | 'due' | 'priority' | 'label' | 'status'
interface TaskGroup {
	key: string
	label: string
	color?: string
	tasks: TaskResponse[]
}

const {t: translate} = useI18n()
const groupBy = useStorage<GroupBy>(() => `webpart.list.groupBy.${props.viewId}`, 'assignee')

const groupByOptions = computed<{value: GroupBy, label: string}[]>(() => [
	{value: 'none', label: translate('webpart.list.groupNone')},
	{value: 'assignee', label: translate('task.attributes.assignees')},
	{value: 'due', label: translate('task.attributes.dueDate')},
	{value: 'priority', label: translate('task.attributes.priority')},
	{value: 'label', label: translate('task.attributes.labels')},
	{value: 'status', label: translate('webpart.list.status')},
])

function startOfDay(date: Date) {
	const d = new Date(date)
	d.setHours(0, 0, 0, 0)
	return d
}

function hexColor(hex?: string | null) {
	if (!hex) return undefined
	return hex.startsWith('#') ? hex : `#${hex}`
}

const groupedTasks = computed<TaskGroup[]>(() => {
	const list = tasks.value
	const groups = new Map<string, TaskGroup>()
	const add = (key: string, label: string, task: TaskResponse, color?: string) => {
		if (!groups.has(key)) groups.set(key, {key, label, color, tasks: []})
		groups.get(key)!.tasks.push(task)
	}

	switch (groupBy.value) {
		case 'assignee':
			for (const task of list) {
				if (!task.assignees?.length) {
					add('unassigned', translate('webpart.list.unassigned'), task)
					continue
				}
				for (const user of task.assignees) {
					add(`user-${user.id}`, user.name || user.username || '', task)
				}
			}
			break
		case 'due': {
			const today = startOfDay(new Date())
			const tomorrow = new Date(today.getTime() + 86400000)
			const weekEnd = new Date(today.getTime() + 7 * 86400000)
			const buckets: [string, string][] = [
				['overdue', translate('task.show.overdue')],
				['today', translate('webpart.list.today')],
				['week', translate('webpart.list.next7Days')],
				['later', translate('webpart.list.later')],
				['none', translate('task.show.noDates')],
			]
			buckets.forEach(([key, label]) => groups.set(key, {key, label, tasks: []}))
			for (const task of list) {
				const due = task.due_date ? new Date(task.due_date) : null
				let key = 'none'
				if (due && due.getTime() > 0) {
					if (due < today && !task.done) key = 'overdue'
					else if (due < tomorrow) key = 'today'
					else if (due < weekEnd) key = 'week'
					else key = 'later'
				}
				groups.get(key)!.tasks.push(task)
			}
			// within a due-date group, soonest first
			for (const group of groups.values()) {
				group.tasks.sort((a, b) => +new Date(a.due_date ?? 0) - +new Date(b.due_date ?? 0))
			}
			break
		}
		case 'priority': {
			const names = ['unset', 'low', 'medium', 'high', 'urgent', 'doNow']
			for (let p = 5; p >= 0; p--) {
				groups.set(`p${p}`, {key: `p${p}`, label: translate(`task.priority.${names[p]}`), tasks: []})
			}
			for (const task of list) {
				groups.get(`p${Math.min(Math.max(task.priority ?? 0, 0), 5)}`)!.tasks.push(task)
			}
			break
		}
		case 'label':
			for (const task of list) {
				if (!task.labels?.length) {
					add('nolabel', translate('webpart.list.noLabel'), task)
					continue
				}
				for (const label of task.labels) {
					add(`label-${label.id}`, label.title ?? '', task, hexColor(label.hex_color))
				}
			}
			break
		case 'status':
			groups.set('open', {key: 'open', label: translate('webpart.list.open'), tasks: []})
			groups.set('done', {key: 'done', label: translate('task.attributes.done'), tasks: []})
			for (const task of list) {
				groups.get(task.done ? 'done' : 'open')!.tasks.push(task)
			}
			break
	}

	const result = [...groups.values()].filter(group => group.tasks.length > 0)
	if (groupBy.value === 'assignee' || groupBy.value === 'label') {
		// named groups alphabetically, the "none" bucket last
		result.sort((a, b) => {
			const aNone = a.key === 'unassigned' || a.key === 'nolabel'
			const bNone = b.key === 'unassigned' || b.key === 'nolabel'
			if (aNone !== bNone) return aNone ? 1 : -1
			return a.label.localeCompare(b.label)
		})
	}
	return result
})

const baseStore = useBaseStore()
const {setDraggedTask} = useTaskDragState()
const {handleTaskDropToProject} = useTaskDragToProject()
const {currentProject: project} = useCurrentProject()

const canWrite = computed(() => {
	return typeof project.value?.max_permission === 'number' &&
		project.value.max_permission > Permissions.READ &&
		project.value.id > 0
})

const isPseudoProject = computed(() => isSavedFilterProject(project.value) || project.value?.id === -1)

onMounted(async () => {
	await nextTick()
	ctaVisible.value = true
})

const canDragTasks = computed(() => canWrite.value || isSavedFilterProject(project.value))

const isTouchDevice = ref(false)
if (typeof window !== 'undefined') {
	isTouchDevice.value = !window.matchMedia('(hover: hover) and (pointer: fine)').matches
}
const dragHandle = computed(() => isTouchDevice.value ? '.handle' : undefined)

const addTaskRef = ref<typeof AddTask | null>(null)

function focusNewTaskInput() {
	addTaskRef.value?.focusTaskInput()
}

function updateTaskList() {
	baseStore.setHasTasks(true)
}

function updateTasks() {
	if (projectId.value < 0) void loadTasks()
}

function handleDragStart(e: { item: HTMLElement }) {
	drag.value = true
	dragTasks.value = [...tasks.value]
	const taskId = parseInt(e.item.dataset.taskId ?? '', 10)
	const task = tasks.value.find(t => t.id === taskId)

	if (task) {
		setDraggedTask(task)
	}
}

async function saveTaskPosition(e: { originalEvent?: MouseEvent, to: HTMLElement, from: HTMLElement, item: HTMLElement, newIndex: number }) {
	drag.value = false
	try {

		// Check if dropped on a sidebar project
		const {moved} = await handleTaskDropToProject(e, (task) => {
			tasks.value = tasks.value.filter(t => t.id !== task.id)
		})

		if (moved) {
			return
		}

		// If dropped outside this list
		if (e.to !== e.from) {
			return
		}

		// e.newIndex is a DOM index: it counts elements still leaving the transition group, so it can
		// point past the last task. The list is already reordered here, so resolve the task by its id.
		const movedTaskId = parseInt(e.item.dataset.taskId ?? '', 10)
		const newIndex = tasks.value.findIndex(t => t.id === movedTaskId)

		if (newIndex === -1) {
			return
		}

		const taskBefore = tasks.value[newIndex - 1] ?? null
		const taskAfter = tasks.value[newIndex + 1] ?? null

		const position = calculateItemPosition(
			taskBefore !== null ? taskBefore.position : null,
			taskAfter !== null ? taskAfter.position : null,
		)

		await positionMutation.mutateAsync({
			position,
			project_view_id: props.viewId,
			taskId: movedTaskId,
		})
	} catch { /* Mutation reports the error. */ } finally {
		dragTasks.value = null
	}
}

const taskRefs = ref<(InstanceType<typeof SingleTaskInProject> | null)[]>([])
const focusedIndex = ref(-1)

function setTaskRef(el: InstanceType<typeof SingleTaskInProject> | null, index: number) {
	if (el === null) {
		delete taskRefs.value[index]
	} else {
		taskRefs.value[index] = el
	}
}

// Webpart: J/K follow the rows as rendered, also when the list is grouped.
const navigableCount = computed(() => groupBy.value === 'none'
	? tasks.value.length
	: groupedTasks.value.reduce((sum, group) => sum + group.tasks.length, 0))

function groupedRowIndex(groupIndex: number, rowIndex: number) {
	let offset = 0
	for (let i = 0; i < groupIndex; i++) offset += groupedTasks.value[i]?.tasks.length ?? 0
	return offset + rowIndex
}

function focusTask(index: number) {
	if (index < 0 || index >= navigableCount.value) {
		return
	}

	const taskRef = taskRefs.value[index]

	focusedIndex.value = index
	taskRef?.focus()
}

function handleListNavigation(e: KeyboardEvent) {
	if (e.target instanceof HTMLElement && (e.target.closest('input, textarea, select, [contenteditable="true"]'))) {
		return
	}

	const shortcut = eventToShortcutString(e)
	if (shortcut === 'KeyJ') {
		e.preventDefault()
		focusTask(Math.min(focusedIndex.value + 1, navigableCount.value - 1))
		return
	}

	if (shortcut === 'KeyK') {
		e.preventDefault()
		if (focusedIndex.value === -1) {
			focusTask(navigableCount.value - 1)
			return
		}

		if (focusedIndex.value === 0) {
			addTaskRef.value?.focusTaskInput()
			focusedIndex.value = -1
			return
		}

		focusTask(Math.max(focusedIndex.value - 1, 0))
		return
	}

	if (shortcut === 'Enter') {
		// Links and buttons activate natively on Enter; leave them alone
		if (e.target instanceof HTMLElement && e.target.closest('a, button, [role="button"]')) {
			return
		}

		// Only act when a row was focused via J/K roving navigation
		if (focusedIndex.value < 0) {
			return
		}

		e.preventDefault()
		taskRefs.value[focusedIndex.value]?.click(e)
	}
}

onMounted(() => {
	document.addEventListener('keydown', handleListNavigation)
})

onBeforeUnmount(() => {
	document.removeEventListener('keydown', handleListNavigation)
})
</script>

<style lang="scss" scoped>
.filter-container {
	display: flex;
	align-items: center;
	gap: .5rem;

	:deep(.popup) {
		max-inline-size: 300px;
	}
}

.tasks {
	padding: .5rem;
}

.task-ghost {
	border-radius: $radius;
	background: var(--grey-100);
	border: 2px dashed var(--grey-300);

	* {
		opacity: 0;
	}
}

.list-view__add-task {
	padding: 1rem 1rem 0;
}

.link-share-view .card {
	border: none;
	box-shadow: none;
}

:deep(.single-task .handle) {
	cursor: grab;
	margin-inline-end: .25rem;
	color: var(--grey-400);
}

@media (hover: hover) and (pointer: fine) {
	:deep(.single-task .handle) {
		display: none;
	}
}

:deep(.tasks:not(.dragging-disabled) .single-task) {
	cursor: grab;
	-webkit-touch-callout: none;
	user-select: none;
	touch-action: manipulation;

	&:active {
		cursor: grabbing;
	}
}

.group-by {
	display: inline-flex;
	align-items: center;
	gap: .4rem;
	block-size: 2.25rem;
	padding: 0 .4rem 0 .75rem;
	border: 1px solid var(--grey-200);
	border-radius: var(--radius-md);
	background: var(--white);
	font-size: .85rem;
	color: var(--text-muted);
}

.group-by__select {
	border: 0;
	background: transparent;
	font: inherit;
	font-weight: 600;
	color: var(--text-strong);
	cursor: pointer;
}

// Webpart: column headings + aligned assignee/due cells
$col-assignees: 7rem;
$col-due: 9.5rem;
$col-icons: 5.5rem;

.list-columns-header {
	display: flex;
	gap: .75rem;
	padding: .75rem 2.5rem .5rem 2.4rem;
	border-block-end: 1px solid var(--grey-200);
	font-size: .7rem;
	font-weight: 600;
	letter-spacing: .05em;
	text-transform: uppercase;
	color: var(--text-muted);

	&__task { flex: 1; }
	&__assignees { flex: 0 0 $col-assignees; }
	&__due { flex: 0 0 $col-due; }
	&__icons { flex: 0 0 $col-icons; }

	@media screen and (max-width: $tablet) {
		display: none;
	}
}

.task-group + .task-group {
	border-block-start: 1px solid var(--grey-200);
}

.task-group__header {
	display: flex;
	align-items: center;
	gap: .5rem;
	margin: 0;
	padding: .9rem 1.25rem .25rem;
	font-size: .85rem;
	font-weight: 600;
	color: var(--text-strong);
}

.task-group__dot {
	inline-size: .55rem;
	block-size: .55rem;
	border-radius: 50%;
}

.task-group__count {
	padding: .05rem .45rem;
	border-radius: 999px;
	background: var(--grey-100);
	color: var(--text-muted);
	font-size: .7rem;
	font-weight: 600;
}

:deep(.single-task.is-columns) {
	@media screen and (min-width: $tablet) {
		.tasktext {
			display: flex;
			align-items: center;
			gap: .75rem;
			flex: 1;
			min-inline-size: 0;

			> span:first-child {
				flex: 0 1 auto;
				min-inline-size: 0;
				overflow: hidden;
				text-overflow: ellipsis;
				white-space: nowrap;
			}

			.labels {
				flex: 1 1 auto;
				min-inline-size: 0;
				flex-wrap: nowrap;
				overflow: hidden;
				margin: 0;
			}
		}

		.cell-assignees {
			flex: 0 0 $col-assignees;
			margin: 0 0 0 auto;
		}

		.cell-due {
			flex: 0 0 $col-due;
			font-size: .8rem;
			color: var(--text-muted);

			.dueDate {
				padding: 0;
			}

			time {
				font-style: normal;
			}
		}

		.cell-icons {
			flex: 0 0 $col-icons;
			display: inline-flex;
			gap: .25rem;
			color: var(--grey-400);
		}

		.cell-empty {
			color: var(--grey-300);
		}
	}
}

.list-view {
	padding-block-end: 1rem;

	:deep(.card) {
		margin-block-end: 0;
	}
}
</style>
