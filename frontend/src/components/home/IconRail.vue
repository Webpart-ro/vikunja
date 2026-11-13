<template>
	<nav
		class="icon-rail d-print-none"
		:aria-label="$t('navigation.main')"
	>
		<RouterLink
			:to="{name: 'home'}"
			class="rail-logo"
			:aria-label="$t('navigation.home')"
		>
			<!-- Webpart mark, same artwork as webpart.ro/app/icon.svg -->
			<svg
				viewBox="0 0 64 64"
				aria-hidden="true"
			>
				<rect
					width="64"
					height="64"
					rx="15"
					fill="#10312b"
				/>
				<path
					fill="#d63e14"
					d="M8 25h7.7l6 12.6L28.2 25h8.6l6.4 12.6L48.8 25H56l-8 18.9h-9.5l-6-11.5-6 11.5H17L8 25Z"
				/>
				<path
					fill="#fff"
					d="M50 16h6l-2.9 7h-6l2.9-7Z"
				/>
			</svg>
		</RouterLink>

		<ul class="rail-list">
			<li
				v-for="item in items"
				:key="item.route"
			>
				<RouterLink
					v-tooltip.right="item.label"
					:to="{name: item.route}"
					class="rail-item"
					:class="{'is-active': isActive(item)}"
					:aria-label="item.label"
				>
					<Icon :icon="item.icon" />
				</RouterLink>
			</li>
		</ul>

		<ul class="rail-list rail-list--bottom">
			<li>
				<RouterLink
					v-tooltip.right="$t('user.settings.title')"
					:to="{name: 'user.settings'}"
					class="rail-item"
					:class="{'is-active': String($route.name ?? '').startsWith('user.settings')}"
					:aria-label="$t('user.settings.title')"
				>
					<Icon icon="cog" />
				</RouterLink>
			</li>
		</ul>
	</nav>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import {useI18n} from 'vue-i18n'
import type {IconProp} from '@fortawesome/fontawesome-svg-core'

import {useConfigStore} from '@/stores/config'
import {PRO_FEATURE} from '@/constants/proFeatures'

interface RailItem {
	route: string
	label: string
	icon: IconProp
	// route-name prefixes that should also mark this item active
	matches?: string[]
}

const {t} = useI18n()
const route = useRoute()
const configStore = useConfigStore()


const items = computed<RailItem[]>(() => {
	const list: RailItem[] = [
		{route: 'home', label: t('navigation.overview'), icon: 'calendar'},
		{route: 'projects.index', label: t('project.projects'), icon: 'layer-group', matches: ['project.']},
		{route: 'labels.index', label: t('label.title'), icon: 'tags', matches: ['labels.']},
		{route: 'teams.index', label: t('team.title'), icon: 'users', matches: ['teams.']},
	]
	if (configStore.isProFeatureEnabled(PRO_FEATURE.TIME_TRACKING)) {
		list.push({route: 'time-tracking', label: t('timeTracking.title'), icon: ['far', 'clock']})
	}
	list.push({route: 'trash', label: t('trash.title'), icon: 'trash-alt'})
	return list
})

function isActive(item: RailItem) {
	const name = String(route.name ?? '')
	if (name === item.route) return true
	return item.matches?.some(prefix => name.startsWith(prefix)) ?? false
}
</script>

<style lang="scss" scoped>
.icon-rail {
	display: none;

	@media screen and (min-width: $tablet) {
		position: fixed;
		inset-block: 0;
		inset-inline-start: 0;
		z-index: 32;
		inline-size: $rail-width;
		padding-block: 1rem;
		background: var(--rail-background);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: .5rem;
	}
}

.rail-logo {
	inline-size: 2.5rem;
	block-size: 2.5rem;
	display: block;
	margin-block-end: .75rem;
	border-radius: 10px;
	// lifts the dark-green mark off the near-black rail
	box-shadow: 0 0 0 1px hsla(0, 0%, 100%, .12);
	transition: transform $transition;

	svg {
		display: block;
		inline-size: 100%;
		block-size: 100%;
	}

	&:hover,
	&:focus-visible {
		transform: scale(1.05);
	}
}

.rail-list {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
	gap: .25rem;

	&--bottom {
		margin-block-start: auto;
	}
}

.rail-item {
	inline-size: 2.75rem;
	block-size: 2.75rem;
	border-radius: 10px;
	color: var(--rail-item-color);
	display: flex;
	align-items: center;
	justify-content: center;
	font-size: 1.05rem;
	transition: background-color $transition, color $transition;

	&:hover,
	&:focus-visible {
		background: var(--rail-item-active-background);
		color: #ffffff;
	}

	&.is-active {
		background: var(--rail-item-active-background);
		color: #ffffff;
	}
}
</style>
