import {computed, defineAsyncComponent, h, ref, shallowRef, watch, type VNode, type AsyncComponentLoader, watchEffect} from 'vue'
import {useRoute, useRouter, type RouteLocationNormalizedGeneric} from 'vue-router'
import {useBaseStore} from '@/stores/base'
import {useProjects} from '@/composables/useProjects'

export function useRouteWithModal() {
	const router = useRouter()
	const route = useRoute()
	const backdropView = computed(() => route.fullPath ? window.history.state?.backdropView : undefined)
	const baseStore = useBaseStore()
	const projectList = useProjects()

	// After a hard reload on a modal route (e.g. /tasks/1 opened from a board) the backdrop
	// route comes from history.state and was never navigated to, so its lazy route
	// components were never loaded. RouterView would render the loader's Promise
	// ("[object Promise]"). Load them first, then let the backdrop render.
	const backdropComponentsLoaded = ref(0)
	watch(backdropView, async (view) => {
		if (!view) return
		const resolved = router.resolve(view)
		let loadedAny = false
		await Promise.all(resolved.matched.flatMap(record =>
			Object.entries(record.components ?? {}).map(async ([name, component]) => {
				if (!isLazyLoader(component)) return
				const mod = await (component as () => Promise<{default?: unknown}>)()
				record.components![name] = (mod?.default ?? mod) as never
				loadedAny = true
			}),
		))
		if (loadedAny) backdropComponentsLoaded.value++
	}, {immediate: true})

	const routeWithModal = computed(() => {
		// re-evaluate once lazy backdrop components have loaded
		void backdropComponentsLoaded.value
		if (!backdropView.value) return route
		const resolved = router.resolve(backdropView.value) as RouteLocationNormalizedGeneric
		const stillLazy = resolved.matched.some(record =>
			Object.values(record.components ?? {}).some(isLazyLoader),
		)
		// render nothing behind the modal until the backdrop's components are ready
		return stillLazy ? {...resolved, matched: []} as RouteLocationNormalizedGeneric : resolved
	})

	const currentModal = shallowRef<VNode>()
	watchEffect(() => {
		if (!backdropView.value) {
			currentModal.value = undefined
			return
		}

		// this is adapted from vue-router
		// https://github.com/vuejs/vue-router-next/blob/798cab0d1e21f9b4d45a2bd12b840d2c7415f38a/src/RouterView.ts#L125
		const routePropsOption = route.matched[0]?.props.default
		let routeProps = undefined
		if (routePropsOption) {
			if (routePropsOption === true) {
				routeProps = route.params
			} else {
				if (typeof routePropsOption === 'function') {
					routeProps = routePropsOption(route)
				} else {
					routeProps = routePropsOption
				}
			}
		}

		if (typeof routeProps === 'undefined') {
			currentModal.value = undefined
			return
		}

		routeProps.backdropView = backdropView.value

		let component = route.matched[0]?.components?.default

		if (typeof component === 'function') {
			component = defineAsyncComponent(component as AsyncComponentLoader)
		}

		if (!component) {
			currentModal.value = undefined
			return
		}
		currentModal.value = h(component, routeProps)
	})

	const historyState = computed(() => route.fullPath ? window.history.state : undefined)

	function closeModal() {
		// If the current project was changed because the user moved the currently opened task while coming from kanban,
		// we need to reflect that change in the route when they close the task modal.
		// The last route is only available as resolved string, therefore we need to use a regex for matching here
		const routeMatch = new RegExp('\\/projects\\/\\d+\\/(\\d+)', 'g')
		const match = historyState.value?.back
			? routeMatch.exec(historyState.value.back)
			: null
		if (match !== null && baseStore.currentProjectId !== 0) {
			let viewId: string | number | undefined = match[1]

			if (!viewId) {
				const project = projectList.projects[baseStore.currentProjectId]
				viewId = project?.views?.[0]?.id
			}

			// Only navigate if we have a valid project and view
			if (viewId) {
				// Preserve query parameters (e.g., date range) from the backdrop view
				const backdropRoute = historyState.value?.backdropView && router.resolve(historyState.value.backdropView)
				const newRoute = {
					name: 'project.view',
					params: {
						projectId: baseStore.currentProjectId,
						viewId,
					},
					query: backdropRoute?.query || {},
				}

				router.push(newRoute)
				return
			}
		}

		// Try browser history first
		if (historyState.value?.back) {
			router.back()
			return
		}

		// Try backdrop view
		const backdropRoute = historyState.value?.backdropView && router.resolve(historyState.value.backdropView)
		if (backdropRoute && backdropRoute.params?.projectId !== '0') {
			router.push(backdropRoute)
			return
		}

		// Fallback to current project or home
		if (baseStore.currentProjectId !== 0) {
			router.push({
				name: 'project.index',
				params: {projectId: baseStore.currentProjectId},
			})
		} else {
			router.push({ name: 'home' })
		}
	}

	return {routeWithModal, currentModal, closeModal}
}

// Same check vue-router uses to tell a lazy `() => import()` loader from a component.
function isLazyLoader(component: unknown): boolean {
	return typeof component === 'function'
		&& !('displayName' in component)
		&& !('props' in component)
		&& !('emits' in component)
		&& !('__vccOpts' in component)
}
