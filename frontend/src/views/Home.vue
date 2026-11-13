<template>
	<div class="dashboard">
		<header class="dashboard__header">
			<h1
				v-if="salutation"
				class="dashboard__title"
			>
				{{ salutation }}
			</h1>
			<AddTask
				class="dashboard__add"
				@tasksAdded="refetchAssigned"
			/>
		</header>

		<Message
			v-if="deletionScheduledAt !== null"
			variant="danger"
			class="mbe-4"
		>
			{{
				$t('user.deletion.scheduled', {
					date: formatDisplayDate(deletionScheduledAt),
					dateSince: formatDateSince(deletionScheduledAt),
				})
			}}
			<RouterLink :to="{name: 'user.settings.deletion'}">
				{{ $t('user.deletion.scheduledCancel') }}
			</RouterLink>
		</Message>

		<ImportHint v-if="assignedQuery.isSuccess.value && !projectList.hasProjects" />

		<!-- Last viewed projects -->
		<section
			v-if="authStore.settings.frontend_settings.show_last_viewed !== false && projectHistory.length > 0"
			class="dashboard__section"
		>
			<h2 class="section-title">
				{{ $t('home.lastViewed') }}
			</h2>
			<div class="project-strip">
				<RouterLink
					v-for="project in projectHistory"
					:key="project.id"
					:to="{name: 'project.index', params: {projectId: project.id}}"
					class="project-tile"
				>
					<span
						class="project-tile__badge"
						:style="{background: colorOf(project.hex_color) || 'var(--primary)'}"
						aria-hidden="true"
					>{{ initialOf(project.title) }}</span>
					<span class="project-tile__body">
						<span class="project-tile__title">{{ getProjectTitle(project) }}</span>
						<span class="project-tile__meta">{{ openCountLabel(project.id) }}</span>
					</span>
					<Icon
						icon="chevron-right"
						class="project-tile__chevron"
					/>
				</RouterLink>
			</div>
		</section>

		<div class="dashboard__grid">
			<!-- Your tickets -->
			<section class="panel panel--tickets">
				<header class="panel__header">
					<h2 class="panel__title">
						{{ $t('webpart.home.yourTickets') }}
					</h2>
					<span class="panel__count">{{ assignedTasks.length }}</span>
				</header>

				<Loading
					v-if="assignedQuery.isPending.value"
					variant="small"
				/>
				<p
					v-else-if="assignedTasks.length === 0"
					class="panel__empty"
				>
					{{ $t('webpart.home.noTickets') }}
				</p>
				<ul
					v-else
					class="ticket-list"
				>
					<li
						v-for="task in assignedTasks"
						:key="task.id"
					>
						<RouterLink
							:to="taskRoute(task.id)"
							class="ticket"
						>
							<span
								class="ticket__dot"
								:style="{background: colorOf(projectOf(task)?.hex_color) || 'var(--grey-300)'}"
								aria-hidden="true"
							/>
							<span class="ticket__main">
								<span class="ticket__title">{{ task.title }}</span>
								<span class="ticket__meta">
									<span class="ticket__id">{{ taskIdentifier(task) }}</span>
									<span v-if="projectOf(task)">· {{ getProjectTitle(projectOf(task)!) }}</span>
								</span>
							</span>
							<span
								v-if="(task.priority ?? 0) >= 3"
								class="chip chip--priority"
							>{{ priorityLabel(task.priority ?? 0) }}</span>
							<span
								v-if="hasDue(task)"
								class="chip"
								:class="dueClass(task)"
							>{{ dueLabel(task) }}</span>
						</RouterLink>
					</li>
				</ul>
			</section>

			<div class="dashboard__side">
				<!-- Deadlines -->
				<section class="panel">
					<header class="panel__header">
						<h2 class="panel__title">
							{{ $t('webpart.home.deadlines') }}
						</h2>
						<span class="panel__hint">{{ $t('webpart.home.next7Days') }}</span>
					</header>
					<p
						v-if="!assignedQuery.isPending.value && deadlineTasks.length === 0"
						class="panel__empty"
					>
						{{ $t('webpart.home.noDeadlines') }}
					</p>
					<ul
						v-else
						class="deadline-list"
					>
						<li
							v-for="task in deadlineTasks"
							:key="task.id"
						>
							<RouterLink
								:to="taskRoute(task.id)"
								class="deadline"
							>
								<span
									class="deadline__date"
									:class="dueClass(task)"
								>
									<span class="deadline__day">{{ dayOf(task.due_date) }}</span>
									<span class="deadline__month">{{ monthOf(task.due_date) }}</span>
								</span>
								<span class="deadline__main">
									<span class="deadline__title">{{ task.title }}</span>
									<span
										class="deadline__when"
										:class="dueClass(task)"
									>{{ dueLabel(task) }}</span>
								</span>
							</RouterLink>
						</li>
					</ul>
				</section>

				<!-- Latest comments -->
				<section class="panel">
					<header class="panel__header">
						<h2 class="panel__title">
							{{ $t('webpart.home.latestComments') }}
						</h2>
					</header>
					<Loading
						v-if="commentsLoading"
						variant="small"
					/>
					<p
						v-else-if="latestComments.length === 0"
						class="panel__empty"
					>
						{{ $t('webpart.home.noComments') }}
					</p>
					<ul
						v-else
						class="comment-list"
					>
						<li
							v-for="item in latestComments"
							:key="item.comment.id"
						>
							<RouterLink
								:to="taskRoute(item.task.id)"
								class="comment"
							>
								<UserAvatar
									:user="item.comment.author"
									:size="28"
									class="comment__avatar"
								/>
								<span class="comment__main">
									<span class="comment__head">
										<strong>{{ getDisplayName(item.comment.author) }}</strong>
										<span class="comment__time">{{ formatDateSince(item.comment.created) }}</span>
									</span>
									<span class="comment__task">{{ item.task.title }}</span>
									<span class="comment__excerpt">{{ excerpt(item.comment.comment) }}</span>
								</span>
							</RouterLink>
						</li>
					</ul>
				</section>
			</div>
		</div>
	</div>
</template>

<script lang="ts" setup>
import {computed, watch} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import {useI18n} from 'vue-i18n'
import {useQuery, useQueries} from '@tanstack/vue-query'

import Message from '@/components/misc/Message.vue'
import Loading from '@/components/misc/Loading.vue'
import UserAvatar from '@/components/misc/UserAvatar.vue'
import AddTask from '@/components/tasks/AddTask.vue'
import ImportHint from '@/components/home/ImportHint.vue'

import {getHistory} from '@/modules/projectHistory'
import {parseDateOrNull} from '@/helpers/parseDateOrNull'
import {parseQuickAddPrefill, QUICK_ADD_PREFILL_PARAMS} from '@/helpers/quickAddPrefill'
import {formatDateSince, formatDisplayDate} from '@/helpers/time/formatDate'
import {getProjectTitle} from '@/helpers/getProjectTitle'
import {getDisplayName} from '@/helpers/user'
import {useDaytimeSalutation} from '@/composables/useDaytimeSalutation'
import {allTasksQuery, type TaskResponse} from '@/client/queries/tasks'
import {commentsQuery} from '@/client/queries/comments'

import {useProjects} from '@/composables/useProjects'
import {useAuthStore} from '@/stores/auth'
import {useBaseStore} from '@/stores/base'

// Webpart dashboard: last viewed projects, the user's tickets, upcoming deadlines
// and the latest comments on tickets assigned to them.

const MAX_TICKETS = 12
const MAX_COMMENT_TASKS = 15
const MAX_COMMENTS = 8
const DAY = 86400000

const salutation = useDaytimeSalutation()
const {t, locale} = useI18n()

const authStore = useAuthStore()
const projectList = useProjects()
const route = useRoute()
const router = useRouter()
const baseStore = useBaseStore()

watch(() => route.query, query => {
	const prefill = parseQuickAddPrefill(query)
	if (prefill === null) {
		return
	}
	baseStore.setQuickActionsActive(true, prefill)
	// Stripped so a reload doesn't open the dialog again.
	const rest = {...query}
	QUICK_ADD_PREFILL_PARAMS.forEach(param => delete rest[param])
	router.replace({name: 'home', query: rest})
}, {immediate: true})

const deletionScheduledAt = computed(() => parseDateOrNull(authStore.info?.deletion_scheduled_at))

const projectHistory = computed(() => {
	// If we don't check this, it tries to load the project background right after logging out
	if (!authStore.authenticated) {
		return []
	}
	return getHistory()
		.map(l => projectList.projects[l.id])
		.filter(l => Boolean(l))
})

// --- Tickets assigned to the current user -------------------------------------------------

const username = computed(() => authStore.info?.username ?? '')

const assignedQuery = useQuery(computed(() => ({
	...allTasksQuery({
		params: {
			filter: `done = false && assignees in ${username.value}`,
			filter_include_nulls: false,
			sort_by: ['due_date', 'id'],
			order_by: ['asc', 'desc'],
		},
	}),
	enabled: username.value !== '',
})))

function refetchAssigned() {
	void assignedQuery.refetch()
}

function dueTime(task: TaskResponse) {
	return task.due_date ? new Date(task.due_date).getTime() : 0
}

function hasDue(task: TaskResponse) {
	return dueTime(task) > 0
}

const allAssigned = computed<TaskResponse[]>(() => assignedQuery.data.value ?? [])

// Dated tickets first (soonest / most overdue on top), then undated by most recently updated.
const assignedTasks = computed(() => [...allAssigned.value]
	.sort((a, b) => {
		const ad = dueTime(a), bd = dueTime(b)
		if (ad && bd) return ad - bd
		if (ad) return -1
		if (bd) return 1
		return +new Date(b.updated ?? 0) - +new Date(a.updated ?? 0)
	})
	.slice(0, MAX_TICKETS))

const deadlineTasks = computed(() => {
	const horizon = Date.now() + 7 * DAY
	return allAssigned.value
		.filter(task => hasDue(task) && dueTime(task) <= horizon)
		.sort((a, b) => dueTime(a) - dueTime(b))
		.slice(0, 6)
})

const openCountByProject = computed(() => {
	const counts: Record<number, number> = {}
	for (const task of allAssigned.value) {
		counts[task.project_id ?? 0] = (counts[task.project_id ?? 0] ?? 0) + 1
	}
	return counts
})

function openCountLabel(projectId: number) {
	const count = openCountByProject.value[projectId] ?? 0
	return count > 0
		? t('webpart.home.assignedToYou', {count}, count)
		: t('webpart.home.openBoard')
}

// --- Latest comments on assigned tickets ---------------------------------------------------

const commentTasks = computed(() => [...allAssigned.value]
	.sort((a, b) => +new Date(b.updated ?? 0) - +new Date(a.updated ?? 0))
	.slice(0, MAX_COMMENT_TASKS))

const commentQueries = useQueries({
	queries: computed(() => commentTasks.value.map(task => commentsQuery(task.id, 'desc', 1))),
})

const commentsLoading = computed(() =>
	assignedQuery.isPending.value || commentQueries.value.some(query => query.isPending),
)

const latestComments = computed(() => {
	const myId = authStore.info?.id
	const items = commentTasks.value.flatMap((task, index) =>
		(commentQueries.value[index]?.data?.items ?? [])
			.filter(comment => comment.author?.id !== myId)
			.map(comment => ({task, comment})),
	)
	return items
		.sort((a, b) => +new Date(b.comment.created ?? 0) - +new Date(a.comment.created ?? 0))
		.slice(0, MAX_COMMENTS)
})

// --- Formatting helpers ----------------------------------------------------------------------

function colorOf(hex?: string | null) {
	if (!hex) return ''
	return hex.startsWith('#') ? hex : `#${hex}`
}

function initialOf(title: string) {
	return (title.trim()[0] ?? '?').toUpperCase()
}

function projectOf(task: TaskResponse) {
	return projectList.projects[task.project_id ?? 0]
}

function taskIdentifier(task: TaskResponse) {
	return task.identifier || `#${task.index ?? task.id}`
}

function taskRoute(id: number) {
	return {name: 'task.detail', params: {id}}
}

function priorityLabel(priority: number) {
	const names = ['unset', 'low', 'medium', 'high', 'urgent', 'doNow']
	return t(`task.priority.${names[Math.min(Math.max(priority, 0), 5)]}`)
}

function startOfDay(date: Date) {
	const d = new Date(date)
	d.setHours(0, 0, 0, 0)
	return d.getTime()
}

function daysUntil(task: TaskResponse) {
	return Math.round((startOfDay(new Date(dueTime(task))) - startOfDay(new Date())) / DAY)
}

function dueLabel(task: TaskResponse) {
	const days = daysUntil(task)
	if (days < 0) return t('webpart.home.overdueBy', {count: -days}, -days)
	if (days === 0) return t('webpart.list.today')
	if (days === 1) return t('webpart.home.tomorrow')
	if (days < 7) return t('webpart.home.inDays', {count: days}, days)
	return new Date(dueTime(task)).toLocaleDateString(locale.value, {day: 'numeric', month: 'short'})
}

function dueClass(task: TaskResponse) {
	const days = daysUntil(task)
	if (days < 0) return 'is-overdue'
	if (days <= 1) return 'is-soon'
	return ''
}

function dayOf(date?: string | null) {
	return date ? new Date(date).getDate() : ''
}

function monthOf(date?: string | null) {
	return date ? new Date(date).toLocaleDateString(locale.value, {month: 'short'}) : ''
}

function excerpt(html: string) {
	const text = new DOMParser().parseFromString(html, 'text/html').body.textContent ?? ''
	const trimmed = text.replace(/\s+/g, ' ').trim()
	return trimmed.length > 140 ? `${trimmed.slice(0, 140)}…` : trimmed
}
</script>

<style scoped lang="scss">
.dashboard {
	max-inline-size: 1200px;
	margin: 0 auto;
	padding-block-end: 2rem;
}

.dashboard__header {
	display: flex;
	flex-direction: column;
	gap: 1rem;
	margin-block-end: 1.75rem;
}

.dashboard__title {
	margin: 0;
	font-size: 1.75rem;
	font-weight: 700;
	letter-spacing: -0.02em;
	color: var(--text-strong);
}

.dashboard__add {
	max-inline-size: none;
}

.dashboard__section {
	margin-block-end: 1.75rem;
}

.section-title {
	margin: 0 0 .75rem;
	font-size: .75rem;
	font-weight: 600;
	letter-spacing: .05em;
	text-transform: uppercase;
	color: var(--text-muted);
}

// Last viewed
.project-strip {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
	gap: .75rem;
}

.project-tile {
	display: flex;
	align-items: center;
	gap: .75rem;
	padding: .85rem 1rem;
	background: var(--white);
	border: 1px solid var(--grey-200);
	border-radius: var(--radius-lg);
	color: var(--text);
	transition: border-color $transition, box-shadow $transition;

	&:hover,
	&:focus-visible {
		border-color: var(--grey-300);
		box-shadow: 0 4px 12px hsla(var(--grey-500-hsl), .12);
		color: var(--text-strong);

		.project-tile__chevron {
			transform: translateX(2px);
			color: var(--primary);
		}
	}
}

.project-tile__badge {
	flex-shrink: 0;
	inline-size: 2.25rem;
	block-size: 2.25rem;
	border-radius: 10px;
	display: flex;
	align-items: center;
	justify-content: center;
	color: #ffffff;
	font-weight: 700;
	text-shadow: 0 1px 1px rgba(0, 0, 0, .25);
}

.project-tile__body {
	flex: 1;
	min-inline-size: 0;
	display: flex;
	flex-direction: column;
	gap: .1rem;
}

.project-tile__title {
	font-weight: 600;
	color: var(--text-strong);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.project-tile__meta {
	font-size: .8rem;
	color: var(--text-muted);
}

.project-tile__chevron {
	inline-size: .7rem;
	color: var(--grey-400);
	transition: transform $transition, color $transition;
}

// Panels
.dashboard__grid {
	display: grid;
	grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
	gap: 1.25rem;
	align-items: start;

	@media screen and (max-width: $desktop) {
		grid-template-columns: minmax(0, 1fr);
	}
}

.dashboard__side {
	display: flex;
	flex-direction: column;
	gap: 1.25rem;
}

.panel {
	background: var(--white);
	border: 1px solid var(--grey-200);
	border-radius: var(--radius-lg);
	overflow: hidden;
}

.panel__header {
	display: flex;
	align-items: center;
	gap: .5rem;
	padding: .9rem 1.1rem;
	border-block-end: 1px solid var(--grey-200);
}

.panel__title {
	margin: 0;
	font-size: .95rem;
	font-weight: 600;
	color: var(--text-strong);
}

.panel__count {
	padding: .05rem .5rem;
	border-radius: 999px;
	background: var(--grey-100);
	color: var(--text-muted);
	font-size: .75rem;
	font-weight: 600;
}

.panel__hint {
	margin-inline-start: auto;
	font-size: .75rem;
	color: var(--text-muted);
}

.panel__empty {
	margin: 0;
	padding: 1.5rem 1.1rem;
	color: var(--text-muted);
	font-size: .875rem;
}

.ticket-list,
.deadline-list,
.comment-list {
	list-style: none;
	margin: 0;
	padding: .35rem;
}

// Tickets
.ticket {
	display: flex;
	align-items: center;
	gap: .75rem;
	padding: .6rem .75rem;
	border-radius: var(--radius-md);
	color: var(--text);

	&:hover,
	&:focus-visible {
		background: var(--grey-50);
	}
}

.ticket__dot {
	flex-shrink: 0;
	inline-size: .55rem;
	block-size: .55rem;
	border-radius: 50%;
}

.ticket__main {
	flex: 1;
	min-inline-size: 0;
	display: flex;
	flex-direction: column;
	gap: .1rem;
}

.ticket__title {
	font-size: .9rem;
	font-weight: 500;
	color: var(--text-strong);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.ticket__meta {
	display: flex;
	gap: .3rem;
	font-size: .75rem;
	color: var(--text-muted);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.ticket__id {
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.chip {
	flex-shrink: 0;
	padding: .15rem .5rem;
	border-radius: var(--radius-sm);
	background: var(--grey-100);
	color: var(--text-muted);
	font-size: .75rem;
	font-weight: 500;
	white-space: nowrap;

	&.is-soon {
		background: hsla(38, 92%, 50%, .14);
		color: hsl(32, 81%, 29%);
	}

	&.is-overdue {
		background: hsla(var(--danger-h), var(--danger-s), 50%, .12);
		color: var(--danger-text);
	}
}

.chip--priority {
	background: hsla(25, 95%, 53%, .12);
	color: hsl(17, 88%, 35%);
}

// Deadlines
.deadline {
	display: flex;
	align-items: center;
	gap: .75rem;
	padding: .5rem .75rem;
	border-radius: var(--radius-md);
	color: var(--text);

	&:hover,
	&:focus-visible {
		background: var(--grey-50);
	}
}

.deadline__date {
	flex-shrink: 0;
	inline-size: 2.6rem;
	padding: .25rem 0;
	border-radius: var(--radius-md);
	border: 1px solid var(--grey-200);
	display: flex;
	flex-direction: column;
	align-items: center;
	line-height: 1.1;

	&.is-soon {
		border-color: hsla(38, 92%, 50%, .5);
		background: hsla(38, 92%, 50%, .08);
	}

	&.is-overdue {
		border-color: hsla(var(--danger-h), var(--danger-s), 50%, .45);
		background: hsla(var(--danger-h), var(--danger-s), 50%, .07);
	}
}

.deadline__day {
	font-size: 1rem;
	font-weight: 700;
	color: var(--text-strong);
}

.deadline__month {
	font-size: .65rem;
	font-weight: 600;
	text-transform: uppercase;
	color: var(--text-muted);
}

.deadline__main {
	min-inline-size: 0;
	display: flex;
	flex-direction: column;
	gap: .1rem;
}

.deadline__title {
	font-size: .875rem;
	font-weight: 500;
	color: var(--text-strong);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.deadline__when {
	font-size: .75rem;
	color: var(--text-muted);

	&.is-soon {
		color: hsl(32, 81%, 29%);
	}

	&.is-overdue {
		color: var(--danger-text);
		font-weight: 600;
	}
}

// Comments
.comment {
	display: flex;
	gap: .65rem;
	padding: .65rem .75rem;
	border-radius: var(--radius-md);
	color: var(--text);

	&:hover,
	&:focus-visible {
		background: var(--grey-50);
	}
}

.comment__avatar {
	flex-shrink: 0;
	border-radius: 50%;
}

.comment__main {
	min-inline-size: 0;
	display: flex;
	flex-direction: column;
	gap: .15rem;
	font-size: .85rem;
}

.comment__head {
	display: flex;
	gap: .4rem;
	align-items: baseline;
	color: var(--text-strong);
}

.comment__time {
	font-size: .75rem;
	color: var(--text-muted);
}

.comment__task {
	font-size: .75rem;
	color: var(--primary);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.comment__excerpt {
	color: var(--text);
	display: -webkit-box;
	-webkit-line-clamp: 2;
	-webkit-box-orient: vertical;
	overflow: hidden;
}
</style>
