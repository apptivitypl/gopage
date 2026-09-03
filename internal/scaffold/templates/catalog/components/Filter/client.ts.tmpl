import type { Props } from "./props.generated";

export function mount(element: HTMLElement, props: Props): () => void {
	const select = element.querySelector("select");
	const button = element.querySelector("button");
	if (!select || !button) {
		return () => {};
	}
	select.value = props.Selected;
	button.hidden = true;
	const onChange = () => {
		const url = new URL(location.href);
		if (select.value) {
			url.searchParams.set("city", select.value);
		} else {
			url.searchParams.delete("city");
		}
		location.href = url.toString();
	};
	select.addEventListener("change", onChange);
	return () => {
		select.removeEventListener("change", onChange);
		button.hidden = false;
	};
}
