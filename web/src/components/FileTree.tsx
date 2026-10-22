import type { TemplateFileData } from "../types";

// A node of the file tree: a directory is a node with children and no file, a
// file is a node with a file and no children.
export interface TreeNode {
  name: string;
  path: string;
  children: TreeNode[];
  file: TemplateFileData | null;
}

// buildTree turns the flat list of paths the API sends into a tree. The API
// stores the paths flat, and the paths are relative to the project the
// template is applied to, so the folders come from the path itself.
export function buildTree(files: TemplateFileData[]): TreeNode[] {
  const root: TreeNode = { name: "", path: "", children: [], file: null };

  for (const file of files) {
    const parts = file.path.split("/").filter((part) => part !== "");
    let node = root;
    let sofar = "";

    parts.forEach((part, index) => {
      sofar = sofar === "" ? part : `${sofar}/${part}`;
      const last = index === parts.length - 1;

      let child = node.children.find((candidate) => candidate.name === part);
      if (child === undefined) {
        child = { name: part, path: sofar, children: [], file: null };
        node.children.push(child);
      }

      if (last) {
        child.file = file;
      }
      node = child;
    });
  }

  sortTree(root.children);
  return root.children;
}

// The folders come first, then the files, and each group is alphabetical.
function sortTree(nodes: TreeNode[]): void {
  nodes.sort((left, right) => {
    const leftIsFolder = left.file === null;
    const rightIsFolder = right.file === null;
    if (leftIsFolder !== rightIsFolder) {
      return leftIsFolder ? -1 : 1;
    }
    return left.name.localeCompare(right.name);
  });

  for (const node of nodes) {
    sortTree(node.children);
  }
}

interface FileTreeProps {
  nodes: TreeNode[];
  selected: string | null;
  onSelect: (file: TemplateFileData) => void;
}

export default function FileTree({ nodes, selected, onSelect }: FileTreeProps) {
  if (nodes.length === 0) {
    return <p className="empty">This template holds no files.</p>;
  }

  return (
    <ul className="tree">
      {nodes.map((node) => {
        const file = node.file;
        return (
          <li key={node.path}>
            {file === null ? (
              <>
                <span className="treedir">{node.name}/</span>
                <FileTree nodes={node.children} selected={selected} onSelect={onSelect} />
              </>
            ) : (
              <button
                type="button"
                className={selected === file.path ? "treefile selected" : "treefile"}
                onClick={() => onSelect(file)}
              >
                {node.name}
                {file.is_executable ? <span className="exec">exec</span> : null}
              </button>
            )}
          </li>
        );
      })}
    </ul>
  );
}
