import { Navigate, redirect, useLocation } from "react-router"

function redirectToSignin(request: Request): never {
  const url = new URL(request.url)
  throw redirect(`/user/signin${url.search}`)
}

export function loader({ request }: { request: Request }) {
  return redirectToSignin(request)
}

export function clientLoader({ request }: { request: Request }) {
  return redirectToSignin(request)
}

export default function SignupRoute() {
  const location = useLocation()
  return <Navigate to={`/user/signin${location.search}`} replace />
}
