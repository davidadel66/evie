import { useEffect, useRef, useState } from "react"
import * as THREE from "three"
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js"
import { GLTFLoader } from "three/examples/jsm/loaders/GLTFLoader.js"
import { MTLLoader } from "three/examples/jsm/loaders/MTLLoader.js"
import { OBJLoader } from "three/examples/jsm/loaders/OBJLoader.js"
import { STLLoader } from "three/examples/jsm/loaders/STLLoader.js"
import { cn } from "./lib/utils.ts"

// 3D model viewer (glTF, GLB, STL and OBJ), kept in its own module so
// three.js and its loaders are dynamically imported only when a 3D model is
// shown (decision 101).
//
// In the conversation, dragging turns the model at any time, while scrolling
// moves the conversation until the viewer is clicked; clicking inside arms
// wheel-zoom until a click outside it (decision 108). Full screen arms zoom
// immediately.

export type ModelDimensions = { x: number; y: number; z: number }

export function ModelView({
  url,
  type,
  full = false,
  resetTick,
  onDimensions,
}: {
  url: string
  type: string
  full?: boolean
  resetTick: number
  onDimensions?: (dims: ModelDimensions | undefined) => void
}) {
  const container = useRef<HTMLDivElement>(null)
  const resetRef = useRef<(() => void) | undefined>(undefined)
  const [zoomArmed, setZoomArmed] = useState(full)
  const zoomArmedRef = useRef(full)
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading")
  const [errorText, setErrorText] = useState<string>()

  useEffect(() => {
    zoomArmedRef.current = full || zoomArmed
  }, [full, zoomArmed])

  // Disarm wheel-zoom when clicking outside the viewer in the conversation.
  useEffect(() => {
    if (full || !zoomArmed) return
    const onDown = (event: PointerEvent) => {
      if (container.current && !container.current.contains(event.target as Node)) setZoomArmed(false)
    }
    document.addEventListener("pointerdown", onDown, true)
    return () => document.removeEventListener("pointerdown", onDown, true)
  }, [full, zoomArmed])

  useEffect(() => {
    if (resetTick > 0) resetRef.current?.()
  }, [resetTick])

  useEffect(() => {
    const host = container.current
    if (!host) return
    let cancelled = false
    setStatus("loading")
    setErrorText(undefined)
    onDimensions?.(undefined)

    const width = host.clientWidth || 640
    const height = host.clientHeight || 420
    const scene = new THREE.Scene()
    scene.background = new THREE.Color(0x131312)

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.01, 10000)
    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false })
    } catch (cause) {
      setErrorText(cause instanceof Error ? cause.message : "WebGL is not available in this browser.")
      setStatus("error")
      return
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
    renderer.setSize(width, height)
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = 1.1
    host.replaceChildren(renderer.domElement)

    // Soft three-point lighting suited to dark ground.
    scene.add(new THREE.HemisphereLight(0xf3efe8, 0x262522, 1.2))
    const key = new THREE.DirectionalLight(0xfff9ee, 1.8)
    key.position.set(5, 8, 6)
    scene.add(key)
    const rim = new THREE.DirectionalLight(0x9db4c8, 0.9)
    rim.position.set(-6, 4, -5)
    scene.add(rim)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.dampingFactor = 0.08
    controls.enableZoom = full || zoomArmedRef.current

    let initialPos = new THREE.Vector3(2, 1.5, 3)
    let initialTarget = new THREE.Vector3(0, 0, 0)
    const resetCamera = () => {
      camera.position.copy(initialPos)
      controls.target.copy(initialTarget)
      controls.update()
    }
    resetRef.current = resetCamera

    // Keep controls.enableZoom in sync with whether the viewer is armed so
    // OrbitControls never swallows page scroll until clicked (decision 108).
    const syncZoom = () => {
      controls.enableZoom = full || zoomArmedRef.current
    }
    const onWheelCapture = () => syncZoom()
    renderer.domElement.addEventListener("wheel", onWheelCapture, { capture: true, passive: true })

    let frame = 0
    const animate = () => {
      frame = requestAnimationFrame(animate)
      controls.enableZoom = full || zoomArmedRef.current
      controls.update()
      renderer.render(scene, camera)
    }
    animate()

    const resize = new ResizeObserver(() => {
      const w = host.clientWidth
      const h = host.clientHeight
      if (w <= 0 || h <= 0) return
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      renderer.setSize(w, h)
    })
    resize.observe(host)

    ;(async () => {
      try {
        const object = await loadObject(url, type)
        if (cancelled) {
          disposeObject(object)
          return
        }
        const box = new THREE.Box3().setFromObject(object)
        if (box.isEmpty()) throw new Error("This 3D file contains no visible geometry.")
        const size = new THREE.Vector3()
        const center = new THREE.Vector3()
        box.getSize(size)
        box.getCenter(center)
        onDimensions?.({ x: size.x, y: size.y, z: size.z })

        // Center the model on the origin and frame it cleanly.
        object.position.sub(center)
        scene.add(object)

        const radius = Math.max(size.length() / 2, 0.001)
        const grid = new THREE.GridHelper(radius * 4, 16, 0x2a2a27, 0x1e1e1c)
        grid.position.y = -size.y / 2
        scene.add(grid)

        const fov = (camera.fov * Math.PI) / 180
        const dist = (radius / Math.sin(fov / 2)) * 1.15
        camera.near = Math.max(dist / 1000, 0.001)
        camera.far = Math.max(dist * 100, 100)
        camera.updateProjectionMatrix()
        initialTarget = new THREE.Vector3(0, 0, 0)
        initialPos = new THREE.Vector3(dist * 0.65, dist * 0.45, dist * 0.75)
        resetCamera()
        setStatus("ready")
      } catch (cause) {
        if (cancelled) return
        setErrorText(cause instanceof Error ? cause.message : String(cause))
        setStatus("error")
      }
    })()

    return () => {
      cancelled = true
      resetRef.current = undefined
      cancelAnimationFrame(frame)
      resize.disconnect()
      renderer.domElement.removeEventListener("wheel", onWheelCapture, { capture: true })
      controls.dispose()
      scene.traverse((node) => {
        if (node instanceof THREE.Mesh) {
          node.geometry?.dispose()
          const mats = Array.isArray(node.material) ? node.material : [node.material]
          for (const mat of mats) mat?.dispose()
        }
      })
      renderer.dispose()
      host.replaceChildren()
    }
  }, [url, type, full])

  if (status === "error") {
    return (
      <div className="flex h-shown-model flex-col items-center justify-center gap-2 rounded-chip bg-bg p-4 text-center text-caption">
        <p className="text-status-failed">Could not load this 3D model.</p>
        {errorText && <p className="font-mono text-text-muted">{errorText}</p>}
      </div>
    )
  }

  return (
    <div
      onClick={() => {
        if (!full) setZoomArmed(true)
      }}
      className={cn("relative overflow-hidden rounded-chip bg-bg", full ? "h-full-height w-full-width" : "h-shown-model w-full")}
    >
      <div ref={container} className="h-full w-full cursor-grab active:cursor-grabbing" />
      {status === "loading" && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-bg/80 text-caption text-text-muted">Loading 3D model…</div>
      )}
      {status === "ready" && !full && (
        <p className="pointer-events-none absolute right-2.5 bottom-2 rounded-sm bg-bg/75 px-2 py-0.5 text-2xs text-text-muted">
          {zoomArmed ? "Scroll to zoom · Click outside to release" : "Drag to turn · Click to enable scroll zoom"}
        </p>
      )}
    </div>
  )
}

async function loadObject(url: string, type: string): Promise<THREE.Object3D> {
  if (type === "model/gltf-binary" || type === "model/gltf+json") {
    const loader = new GLTFLoader()
    const gltf = await loader.loadAsync(url)
    return gltf.scene
  }
  if (type === "model/stl") {
    const loader = new STLLoader()
    const geometry = await loader.loadAsync(url)
    geometry.computeVertexNormals()
    const material = new THREE.MeshStandardMaterial({ color: 0xd8d3c8, roughness: 0.48, metalness: 0.15 })
    return new THREE.Mesh(geometry, material)
  }
  if (type === "model/obj") {
    // Check if the OBJ declares an `mtllib` and try loading its materials from the same pass folder.
    const objLoader = new OBJLoader()
    const text = await (await fetch(url)).text()
    const mtlMatch = /^\s*mtllib\s+(.+)$/im.exec(text)
    if (mtlMatch) {
      const mtlName = mtlMatch[1]!.trim()
      const base = url.slice(0, url.lastIndexOf("/") + 1)
      try {
        const mtlLoader = new MTLLoader()
        mtlLoader.setResourcePath(base)
        const materials = await mtlLoader.loadAsync(`${base}${encodeURIComponent(mtlName)}`)
        materials.preload()
        objLoader.setMaterials(materials)
      } catch {
        // Optional material file missing or invalid — fall back to default shaded material.
      }
    }
    const group = objLoader.parse(text)
    // Apply a neutral Dusk material to any mesh that arrived without textures/materials.
    const fallback = new THREE.MeshStandardMaterial({ color: 0xd8d3c8, roughness: 0.5, metalness: 0.12 })
    group.traverse((child) => {
      if (child instanceof THREE.Mesh && !child.material) child.material = fallback
    })
    return group
  }
  throw new Error(`Unsupported 3D format: ${type}`)
}

function disposeObject(object: THREE.Object3D) {
  object.traverse((node) => {
    if (node instanceof THREE.Mesh) {
      node.geometry?.dispose()
      const mats = Array.isArray(node.material) ? node.material : [node.material]
      for (const mat of mats) mat?.dispose()
    }
  })
}

export function formatDimensions(dims: ModelDimensions): string {
  const fmt = (n: number) => (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2)).replace(/\.0+$/, "")
  return `${fmt(dims.x)} × ${fmt(dims.y)} × ${fmt(dims.z)}`
}
