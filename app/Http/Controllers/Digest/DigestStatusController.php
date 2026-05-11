<?php

namespace App\Http\Controllers\Digest;

use App\Http\Controllers\Controller;
use App\Http\Requests\Digest\StatusRequest;
use App\Repositories\DigestStatusRepository;
use Carbon\CarbonImmutable;
use Illuminate\Http\JsonResponse;

class DigestStatusController extends Controller
{
    public function __construct(private readonly DigestStatusRepository $repository) {}

    public function __invoke(StatusRequest $request): JsonResponse
    {
        $since = CarbonImmutable::parse($request->validated()['since']);
        $data = $this->repository->forUser($request->user(), $since);

        return response()->json($data);
    }
}
