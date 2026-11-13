export function setTitle(title : undefined | string) {
	document.title = (typeof title === 'undefined' || title === '')
		? 'Webpart Tasks'
		: `${title} | Webpart Tasks`
}
