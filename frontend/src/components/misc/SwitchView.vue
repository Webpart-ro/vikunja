<template>
	<component
		:is="is"
		class="switch-view"
	>
		<slot />
	</component>
</template>

<script setup lang="ts">
import type {Component} from 'vue'

withDefaults(defineProps<{
	is?: string | Component,
}>(), {
	is: 'div',
})
</script>

<style lang="scss" scoped>
// Webpart: underline tabs instead of a floating pill box
.switch-view {
	display: inline-flex;
	flex-wrap: wrap;
	gap: .25rem;
	font-size: .875rem;
	border-block-end: 1px solid var(--grey-200);
}

::v-slotted(.switch-view-button) {
	padding: .6rem .75rem;
	display: block;
	white-space: nowrap;
	color: var(--text-muted);
	border-block-end: 2px solid transparent;
	margin-block-end: -1px;
	transition: color 100ms, border-color 100ms;

	&:hover {
		color: var(--text-strong);
	}

	&.is-active {
		color: var(--text-strong);
		font-weight: 600;
		border-block-end-color: var(--primary);
	}
}
</style>
