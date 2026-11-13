<template>
	<Dropdown class="project-switcher">
		<template #trigger="{toggleOpen, open}">
			<BaseButton
				class="project-switcher__trigger"
				:aria-expanded="open"
				aria-haspopup="menu"
				:title="$t('project.projects')"
				@click="toggleOpen"
			>
				<span
					v-if="currentColor"
					class="project-switcher__dot"
					:style="{background: currentColor}"
					aria-hidden="true"
				/>
				<span class="project-switcher__title">{{ title }}</span>
				<Icon
					icon="chevron-down"
					class="project-switcher__chevron"
					:class="{'is-open': open}"
				/>
			</BaseButton>
		</template>

		<template #default="{close}">
			<div class="project-switcher__menu">
				<template
					v-for="group in groups"
					:key="group.label"
				>
					<div
						v-if="group.items.length"
						class="project-switcher__group"
					>
						{{ group.label }}
					</div>
					<RouterLink
						v-for="item in group.items"
						:key="`${group.label}-${item.project.id}`"
						:to="{name: 'project.index', params: {projectId: item.project.id}}"
						class="project-switcher__item"
						:class="{'is-active': item.project.id === currentProject?.id}"
						:style="{'--depth': item.depth}"
						@click="close()"
					>
						<span
							class="project-switcher__dot"
							:style="{background: colorOf(item.project) || 'var(--grey-300)'}"
							aria-hidden="true"
						/>
						<span class="project-switcher__item-title">{{ getProjectTitle(item.project) }}</span>
						<Icon
							v-if="item.project.id === currentProject?.id"
							icon="check"
							class="project-switcher__check"
						/>
					</RouterLink>
				</template>

				<RouterLink
					:to="{name: 'projects.index'}"
					class="project-switcher__all"
					@click="close()"
				>
					{{ $t('project.projects') }} →
				</RouterLink>
			</div>
		</template>
	</Dropdown>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import BaseButton from '@/components/base/BaseButton.vue'
import Dropdown from '@/components/misc/Dropdown.vue'
import {useProjects} from '@/composables/useProjects'
import {useCurrentProject} from '@/composables/useCurrentProject'
import {getProjectTitle} from '@/helpers/getProjectTitle'
import type {ProjectResponse} from '@/client/queries/projects'

interface Item {
	project: ProjectResponse
	depth: number
}

const {t} = useI18n()
const projectList = useProjects()
const {currentProject} = useCurrentProject()

function colorOf(project?: {hex_color?: string | null} | null) {
	const hex = project?.hex_color ?? ''
	if (!hex) return ''
	return hex.startsWith('#') ? hex : `#${hex}`
}

const title = computed(() => {
	const project = currentProject.value
	if (!project || project.title === '') return t('misc.loading')
	return getProjectTitle(project)
})

const currentColor = computed(() => colorOf(currentProject.value))

// Root projects followed by their (nested) children, indented by depth.
const projectItems = computed<Item[]>(() => {
	const out: Item[] = []
	const walk = (projects: ProjectResponse[], depth: number) => {
		for (const project of projects) {
			if (project.is_archived) continue
			out.push({project, depth})
			walk(projectList.getChildProjects(project.id), depth + 1)
		}
	}
	walk(projectList.notArchivedRootProjects, 0)
	return out
})

const groups = computed(() => [
	{
		label: t('navigation.savedFilters'),
		items: (projectList.savedFilterProjects as ProjectResponse[])
			.filter(project => !project.is_archived)
			.map(project => ({project, depth: 0})),
	},
	{label: t('project.projects'), items: projectItems.value},
])
</script>

<style lang="scss" scoped>
.project-switcher {
	display: flex;
	align-items: center;
	min-inline-size: 0;
}

.project-switcher__trigger {
	display: inline-flex;
	align-items: center;
	gap: .6rem;
	min-inline-size: 0;
	padding: .35rem .6rem;
	margin-inline-start: -.6rem;
	border-radius: var(--radius-md);
	color: var(--text-strong);
	transition: background-color $transition;

	&:hover,
	&:focus-visible,
	&[aria-expanded="true"] {
		background: var(--grey-100);
	}
}

.project-switcher__title {
	font-size: 1rem;
	font-weight: 700;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;

	@media screen and (min-width: $tablet) {
		font-size: 1.25rem;
		letter-spacing: -0.01em;
	}
}

.project-switcher__chevron {
	flex-shrink: 0;
	inline-size: .8rem;
	color: var(--grey-500);
	transition: transform $transition;

	&.is-open {
		transform: rotate(180deg);
	}
}

.project-switcher__dot {
	flex-shrink: 0;
	inline-size: .6rem;
	block-size: .6rem;
	border-radius: 50%;
}

.project-switcher__menu {
	// sits above Dropdown's hover-bridge ::before, which is absolutely positioned
	position: relative;
	min-inline-size: 280px;
	max-inline-size: 360px;
	max-block-size: min(70vh, 520px);
	overflow-y: auto;
	padding: .35rem;
}

.project-switcher__group {
	padding: .6rem .6rem .3rem;
	font-size: .7rem;
	font-weight: 600;
	letter-spacing: .05em;
	text-transform: uppercase;
	color: var(--text-muted);
}

.project-switcher__item {
	display: flex;
	align-items: center;
	gap: .6rem;
	min-block-size: 2.5rem;
	padding: .4rem .6rem;
	padding-inline-start: calc(.6rem + var(--depth, 0) * 1rem);
	border-radius: var(--radius-md);
	color: var(--text);
	font-size: .9rem;

	&:hover,
	&:focus-visible {
		background: var(--grey-100);
		color: var(--text-strong);
	}

	&.is-active {
		background: hsla(var(--primary-hsl), .1);
		color: var(--primary);
		font-weight: 600;
	}
}

.project-switcher__item-title {
	flex: 1;
	min-inline-size: 0;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.project-switcher__check {
	inline-size: .8rem;
}

.project-switcher__all {
	display: block;
	margin-block-start: .35rem;
	padding: .6rem;
	border-block-start: 1px solid var(--grey-200);
	font-size: .85rem;
	font-weight: 600;
	color: var(--primary);
}
</style>
