import {queryOptions, useMutation} from '@tanstack/vue-query'

import {
	projectGithubReposCreate,
	projectGithubReposDelete,
	projectGithubReposList,
	taskDevelopmentBases,
	taskDevelopmentBranchCreate,
	taskDevelopmentRead,
	type BranchBasesBody,
	type ProjectGithubRepo,
	type TaskDevelopmentBody,
} from '@/client/generated'
import {contextMutationOptions} from './contextMutation'
import {i18n} from '@/i18n'

// Webpart GitHub integration: repositories linked to a project, and the branches / pull
// requests / CI of a task (matched on its identifier, e.g. HS-334).

export const githubKeys = {
	projectRepos: (projectId: number) => ['github', 'project-repos', projectId] as const,
	taskDevelopment: (taskId: number) => ['github', 'task-development', taskId] as const,
	branchBases: (taskId: number, repo: string) => ['github', 'branch-bases', taskId, repo] as const,
}

export function projectGithubReposQuery(projectId: number) {
	return queryOptions({
		queryKey: githubKeys.projectRepos(projectId),
		queryFn: async ({signal}): Promise<ProjectGithubRepo[]> =>
			(await projectGithubReposList({path: {project: projectId}, signal})).data.items ?? [],
		enabled: projectId > 0,
	})
}

export function taskDevelopmentQuery(taskId: number) {
	return queryOptions({
		queryKey: githubKeys.taskDevelopment(taskId),
		queryFn: async ({signal}): Promise<TaskDevelopmentBody> =>
			(await taskDevelopmentRead({path: {id: taskId}, signal})).data,
		enabled: taskId > 0,
		// The card renders its own unavailable state; GitHub being unreachable isn't worth a toast.
		meta: {handlesError: true},
		staleTime: 30_000,
	})
}

export function branchBasesQuery(taskId: number, repo: string) {
	return queryOptions({
		queryKey: githubKeys.branchBases(taskId, repo),
		queryFn: async ({signal}): Promise<BranchBasesBody> =>
			(await taskDevelopmentBases({path: {id: taskId}, query: {repo}, signal})).data,
		enabled: taskId > 0 && repo !== '',
	})
}

export function createGithubRepoMutationOptions() {
	return contextMutationOptions({
		mutationFn: async (input: {projectId: number, owner: string, repo: string}) =>
			(await projectGithubReposCreate({
				path: {project: input.projectId},
				body: {owner: input.owner, repo: input.repo},
			})).data,
		onSettled: (input, client) => client.invalidateQueries({queryKey: githubKeys.projectRepos(input.projectId)}),
		successMessage: () => i18n.global.t('webpart.github.repoLinked'),
	})
}

export function useCreateGithubRepoMutation() {
	return useMutation(createGithubRepoMutationOptions())
}

export function deleteGithubRepoMutationOptions() {
	return contextMutationOptions({
		mutationFn: async (input: {projectId: number, repoId: number}) => {
			await projectGithubReposDelete({path: {project: input.projectId, repo: input.repoId}})
		},
		onSettled: (input, client) => client.invalidateQueries({queryKey: githubKeys.projectRepos(input.projectId)}),
		successMessage: () => i18n.global.t('webpart.github.repoUnlinked'),
	})
}

export function useDeleteGithubRepoMutation() {
	return useMutation(deleteGithubRepoMutationOptions())
}

export function createBranchMutationOptions() {
	return contextMutationOptions({
		mutationFn: async (input: {taskId: number, repo: string, base: string, name: string}) =>
			(await taskDevelopmentBranchCreate({
				path: {id: input.taskId},
				body: {repo: input.repo, base: input.base, name: input.name},
			})).data,
		onSettled: (input, client) => client.invalidateQueries({queryKey: githubKeys.taskDevelopment(input.taskId)}),
		successMessage: (data) => i18n.global.t('webpart.github.branchCreated', {name: data.name}),
	})
}

export function useCreateBranchMutation() {
	return useMutation(createBranchMutationOptions())
}
